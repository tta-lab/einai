package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/prompt"
	"github.com/tta-lab/einai/internal/session"
)

var askCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask a question using the agent loop",
	Long: `Ask a question using an AI agent loop.

With no flags, asks about the current directory with both filesystem and web access.
Use a flag to narrow the scope:

  --project <alias>    Ask about a registered ttal project
  --repo <org/repo>    Ask about a GitHub repo (auto-clone/pull)
  --url <url>          Ask about a web page (fetched with defuddle)
  --web                Search the web to answer

Prompt can be piped via stdin, provided as argument, or both.

Examples:
  ei ask "how does the auth middleware work?"
  ei ask "how does routing work?" --project myapp
  ei ask "explain the pipeline syntax" --repo woodpecker-ci/woodpecker
  ei ask "what auth methods?" --url https://docs.example.com
  ei ask "latest Go generics syntax?" --web
  ei ask "summarize this project" --save
  cat document.txt | ei ask "explain this"`,
	RunE: runAsk,
}

var askFlags struct {
	project string
	repo    string
	url     string
	web     bool
	save    bool
	async   bool
}

func init() {
	askCmd.Flags().StringVar(&askFlags.project, "project", "", "Ask about a registered ttal project")
	askCmd.Flags().StringVar(&askFlags.repo, "repo", "", "Ask about a GitHub/Forgejo repo (auto-clone)")
	askCmd.Flags().StringVar(&askFlags.url, "url", "", "Ask about a web page")
	askCmd.Flags().BoolVar(&askFlags.web, "web", false, "Search the web to answer")
	askCmd.Flags().BoolVar(&askFlags.save, "save", false, "Save the final answer to flicknote")
	askCmd.Flags().BoolVar(&askFlags.async, "async", false,
		"Submit as async pueue job instead of running synchronously")
	_ = askCmd.RegisterFlagCompletionFunc("project", projectCompletion)
	rootCmd.AddCommand(askCmd)
}

// projectCompletion returns shell completions for --project flag using ttal project list
func projectCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	out, err := exec.Command("ttal", "project", "list", "--json").Output()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var projects []struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(out, &projects); err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var aliases []string
	for _, p := range projects {
		aliases = append(aliases, p.Alias)
	}
	return aliases, cobra.ShellCompDirectiveNoFileComp
}

func runAsk(cmd *cobra.Command, args []string) error {
	question, err := readQuestion(args)
	if err != nil {
		return err
	}

	mode, err := resolveAskMode()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}
	req := session.AskRequest{
		Question:   question,
		Mode:       mode,
		Project:    askFlags.project,
		Repo:       askFlags.repo,
		URL:        askFlags.url,
		WorkingDir: cwd,
	}

	if askFlags.async {
		req.Async = true
		req.TmuxTarget = captureTmuxTarget()
		req.Save = askFlags.save
		_, err := blockingEndpoint[session.AskResponse](cmd.Context(), "ask", req)
		if err != nil {
			return err
		}
		fmt.Println("Queued. You'll be notified here when it completes.")
		return nil
	}

	resp, err := blockingEndpoint[session.AskResponse](cmd.Context(), "ask", req)
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return fmt.Errorf("%s", resp.Error)
	}

	if err := renderResult(resp.Result); err != nil {
		return err
	}

	if askFlags.save && resp.Result != "" {
		if saveErr := saveAskResponse(resp.Result); saveErr != nil {
			return saveErr
		}
	}

	return nil
}

// saveAskResponse saves the response to flicknote using the flicknote CLI.
func saveAskResponse(response string) error {
	cmd := exec.Command("flicknote", "add")
	cmd.Stdin = bytes.NewReader([]byte(response))

	output, err := cmd.Output()
	if err != nil {
		if execErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("flicknote exited with code %d: %s", execErr.ExitCode(), strings.TrimSpace(string(execErr.Stderr)))
		}
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "warning: flicknote not found in PATH, skipping save\n")
			return nil
		}
		return fmt.Errorf("flicknote add: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

func resolveAskMode() (prompt.Mode, error) {
	set := 0
	if askFlags.project != "" {
		set++
	}
	if askFlags.repo != "" {
		set++
	}
	if askFlags.url != "" {
		set++
	}
	if askFlags.web {
		set++
	}
	if set > 1 {
		return "", fmt.Errorf("only one of --project, --repo, --url, --web may be specified")
	}
	switch {
	case askFlags.project != "":
		return prompt.ModeProject, nil
	case askFlags.repo != "":
		return prompt.ModeRepo, nil
	case askFlags.url != "":
		return prompt.ModeURL, nil
	case askFlags.web:
		return prompt.ModeWeb, nil
	default:
		return prompt.ModeGeneral, nil
	}
}

func buildQuestion(args []string) string {
	var stdinContent string

	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil {
			stdinContent = string(data)
		}
	}

	positional := ""
	if len(args) > 0 {
		positional = args[0]
	}

	if stdinContent != "" && positional != "" {
		return stdinContent + "\n\n" + positional
	}
	if stdinContent != "" {
		return stdinContent
	}
	if positional != "" {
		return positional
	}
	return ""
}

func readQuestion(args []string) (string, error) {
	question := buildQuestion(args)
	if question != "" {
		return question, nil
	}
	return "", fmt.Errorf("question required — pass as argument or pipe via stdin")
}
