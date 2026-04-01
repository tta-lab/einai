package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/prompt"
	"github.com/tta-lab/einai/internal/session"
)

var askCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask a question using the agent loop",
	Long: `Ask a question about a project, repo, URL, or the web.

Examples:
  ei ask "how does routing work?" --project myapp
  ei ask "what is the latest syntax?" --web
  ei ask --url https://docs.example.com "what auth methods are supported?"`,
	RunE: runAsk,
}

var askFlags struct {
	project   string
	repo      string
	url       string
	web       bool
	maxSteps  int
	maxTokens int
}

func init() {
	askCmd.Flags().StringVar(&askFlags.project, "project", "", "Ask about a registered ttal project")
	askCmd.Flags().StringVar(&askFlags.repo, "repo", "", "Ask about a GitHub/Forgejo repo (auto-clone)")
	askCmd.Flags().StringVar(&askFlags.url, "url", "", "Ask about a web page")
	askCmd.Flags().BoolVar(&askFlags.web, "web", false, "Search the web to answer")
	askCmd.Flags().IntVar(&askFlags.maxSteps, "max-steps", 0, "Maximum agent steps (0 = config default)")
	askCmd.Flags().IntVar(&askFlags.maxTokens, "max-tokens", 0, "Maximum output tokens (0 = config default)")
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
		MaxSteps:   askFlags.maxSteps,
		MaxTokens:  askFlags.maxTokens,
		WorkingDir: cwd,
	}

	return streamEndpoint(cmd.Context(), "ask", req, "ask failed")
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

func readQuestion(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	// Try reading from stdin pipe.
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}
	return "", fmt.Errorf("question required — pass as argument or pipe via stdin")
}
