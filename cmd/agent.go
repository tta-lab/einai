package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/event"
	"github.com/tta-lab/einai/internal/session"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage and run agents",
}

var agentRunCmd = &cobra.Command{
	Use:   "run <name> [prompt]",
	Short: "Run an agent with a prompt",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runAgent,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agents",
	RunE:  runAgentList,
}

var agentFlags struct {
	project   string
	repo      string
	maxSteps  int
	maxTokens int
	env       []string
	workingDir string
}

func init() {
	agentRunCmd.Flags().StringVar(&agentFlags.project, "project", "", "Run in a registered project directory")
	agentRunCmd.Flags().StringVar(&agentFlags.repo, "repo", "", "Run in a cloned repo (read-only)")
	agentRunCmd.Flags().IntVar(&agentFlags.maxSteps, "max-steps", 0, "Maximum agent steps")
	agentRunCmd.Flags().IntVar(&agentFlags.maxTokens, "max-tokens", 0, "Maximum output tokens")
	agentRunCmd.Flags().StringArrayVar(&agentFlags.env, "env", nil, "Extra env vars (KEY=VALUE)")
	agentCmd.AddCommand(agentRunCmd)
	agentCmd.AddCommand(agentListCmd)
	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	name := args[0]

	var agentPrompt string
	if len(args) > 1 {
		agentPrompt = args[1]
	} else {
		stat, err := os.Stdin.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			agentPrompt = string(data)
		}
	}
	if agentPrompt == "" {
		return fmt.Errorf("prompt required — pass as argument or pipe via stdin")
	}

	sandboxEnv := make(map[string]string)
	for _, kv := range agentFlags.env {
		k, v, _ := strings.Cut(kv, "=")
		sandboxEnv[k] = v
	}

	cwd, _ := os.Getwd()
	if agentFlags.workingDir != "" {
		cwd = agentFlags.workingDir
	}

	req := session.AgentRequest{
		Name:       name,
		Prompt:     agentPrompt,
		Project:    agentFlags.project,
		Repo:       agentFlags.repo,
		MaxSteps:   agentFlags.maxSteps,
		MaxTokens:  agentFlags.maxTokens,
		SandboxEnv: sandboxEnv,
		WorkingDir: cwd,
	}

	return streamAgentRun(cmd.Context(), req)
}

func streamAgentRun(ctx context.Context, req session.AgentRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	client := newUnixClient()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://einai/agent/run", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("daemon unreachable (is 'ei daemon run' running?): %w", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e event.Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		switch e.Type {
		case event.EventDelta:
			fmt.Print(e.Text)
		case event.EventStatus:
			fmt.Fprintf(os.Stderr, "\n[%s]\n", e.Message)
		case event.EventError:
			fmt.Fprintf(os.Stderr, "\nError: %s\n", e.Message)
			return fmt.Errorf("agent run failed: %s", e.Message)
		case event.EventDone:
			fmt.Println()
		}
	}
	return scanner.Err()
}

func runAgentList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	agents, err := agent.Discover(cfg.AgentsPaths)
	if err != nil {
		return fmt.Errorf("discover agents: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("No agents found. Configure agents_paths in ~/.config/einai/config.toml")
		return nil
	}

	for _, a := range agents {
		emoji := a.Frontmatter.Emoji
		if emoji == "" {
			emoji = "🤖"
		}
		fmt.Printf("%s %-20s %s\n", emoji, a.Frontmatter.Name, a.Frontmatter.Description)
	}
	return nil
}
