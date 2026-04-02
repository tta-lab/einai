package cmd

import (
	"bytes"
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

The --instruction flag adds a user instruction that is combined with the question
or stdin content. Both stdin and positional arguments can coexist.

MaxSteps and MaxTokens are configured in ~/.config/einai/config.toml.

Examples:
  ei ask "how does the auth middleware work?"
  ei ask "how does routing work?" --project myapp
  ei ask "explain the pipeline syntax" --repo woodpecker-ci/woodpecker
  ei ask "what auth methods?" --url https://docs.example.com
  ei ask "latest Go generics syntax?" --web
  ei ask "summarize this project" --save
  cat document.txt | ei ask --instruction "explain this"
  ei ask "file.txt" --instruction "analyze this file"`,
	RunE: runAsk,
}

var askFlags struct {
	project     string
	repo        string
	url         string
	web         bool
	save        bool
	instruction string
}

func init() {
	askCmd.Flags().StringVar(&askFlags.project, "project", "", "Ask about a registered ttal project")
	askCmd.Flags().StringVar(&askFlags.repo, "repo", "", "Ask about a GitHub/Forgejo repo (auto-clone)")
	askCmd.Flags().StringVar(&askFlags.url, "url", "", "Ask about a web page")
	askCmd.Flags().BoolVar(&askFlags.web, "web", false, "Search the web to answer")
	askCmd.Flags().BoolVar(&askFlags.save, "save", false, "Save the final answer to flicknote")
	askCmd.Flags().StringVarP(&askFlags.instruction, "instruction", "i", "", "Additional instruction to add to the question or stdin")
	_ = askCmd.RegisterFlagCompletionFunc("project", projectCompletion)
	rootCmd.AddCommand(askCmd)
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

	response, err := streamEndpoint(cmd.Context(), "ask", req, "ask failed")
	if err != nil {
		return err
	}

	if askFlags.save && response != "" {
		if saveErr := saveAskResponse(response); saveErr != nil {
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

// isStdinPiped returns true if stdin is being piped (not a terminal).
func isStdinPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// readStdin reads content from stdin if it's piped.
func readStdin() (string, error) {
	if !isStdinPiped() {
		return "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	return string(data), nil
}

// buildQuestion combines positional args, stdin, and --instruction into a single question.
// Priority: instruction is always appended. Stdin is used as context if present.
// If no positional, stdin becomes the base; otherwise positional is the base.
func buildQuestion(args []string, stdinContent, instruction string) string {
	// Determine base content (positional or stdin)
	var base string
	if len(args) > 0 {
		base = args[0]
	} else if stdinContent != "" {
		base = stdinContent
	}

	// Combine with instruction
	if instruction != "" {
		if base != "" {
			return base + "\n\n" + instruction
		}
		return instruction
	}

	return base
}

func readQuestion(args []string) (string, error) {
	stdinContent, _ := readStdin()

	question := buildQuestion(args, stdinContent, askFlags.instruction)
	if question != "" {
		return question, nil
	}

	return "", fmt.Errorf("question required — pass as argument, pipe via stdin, or use --instruction")
}
