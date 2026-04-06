package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/session"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage and run agents",
}

var agentRunCmd = &cobra.Command{
	Use:   "run <name> [prompt]",
	Short: "Run an agent with a prompt",
	Long: `Run a named agent using its frontmatter configuration.

Runtime is determined by: --runtime flag > config default_runtime > "claude-code".

Prompt can be piped via stdin, provided as argument, or both. When both are
provided, they are combined (stdin content + positional instruction).

Examples:
  ei agent run coder "implement the auth module"
  ei agent run coder --runtime ei-native "implement auth"
  cat plan.md | ei agent run coder
  cat plan.md | ei agent run coder "implement this plan"`,
	Args:              cobra.RangeArgs(1, 2),
	RunE:              runAgent,
	ValidArgsFunction: agentNameCompletion,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agents",
	RunE:  runAgentList,
}

var agentSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync agents to Claude Code format",
	RunE:  runAgentSync,
}

var agentFlags struct {
	env     []string
	runtime string
	async   bool
}

var agentSyncFlags struct {
	dryRun bool
	target string
}

func init() {
	agentRunCmd.Flags().StringArrayVar(&agentFlags.env, "env", nil, "Extra env vars (KEY=VALUE)")
	agentRunCmd.Flags().StringVar(&agentFlags.runtime, "runtime", "",
		"Runtime: ei-native or claude-code (default: config or claude-code)")
	agentRunCmd.Flags().BoolVar(&agentFlags.async, "async", false,
		"Submit as async pueue job instead of running synchronously")
	agentSyncCmd.Flags().BoolVar(&agentSyncFlags.dryRun, "dry-run", false, "Show what would be written without writing")
	agentSyncCmd.Flags().StringVar(&agentSyncFlags.target, "target", "", "Target directory (default ~/.claude/agents)")
	agentCmd.AddCommand(agentRunCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentSyncCmd)
	rootCmd.AddCommand(agentCmd)
}

// agentNameCompletion provides shell completion for agent names
func agentNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	agents, err := agent.Discover(cfg.AgentsPaths)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, a := range agents {
		names = append(names, a.Frontmatter.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func runAgent(cmd *cobra.Command, args []string) error {
	name := args[0]
	agentPrompt := buildPrompt(args)

	if agentPrompt == "" {
		return fmt.Errorf("prompt required — pass as argument or pipe via stdin")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}

	sandboxEnv := make(map[string]string)
	for _, kv := range agentFlags.env {
		k, v, _ := strings.Cut(kv, "=")
		sandboxEnv[k] = v
	}

	req := session.AgentRequest{
		Name:       name,
		Prompt:     agentPrompt,
		SandboxEnv: sandboxEnv,
		WorkingDir: cwd,
		Runtime:    agentFlags.runtime,
	}

	if agentFlags.async {
		req.Async = true
		req.SendTarget = captureSendTarget()
		_, err := blockingEndpoint[session.AgentResponse](cmd.Context(), "agent/run", req)
		if err != nil {
			return err
		}
		fmt.Println("Queued. You'll be notified here when it completes.")
		return nil
	}

	resp, err := blockingEndpoint[session.AgentResponse](cmd.Context(), "agent/run", req)
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return fmt.Errorf("%s", resp.Error)
	}
	return renderResult(resp.Result)
}

// captureSendTarget returns the ttal send target for the current session.
// Returns "jobID:agentName" for worker sessions (both TTAL_JOB_ID and
// TTAL_AGENT_NAME set), "agentName" for manager sessions (TTAL_AGENT_NAME only),
// and "" with a stderr warning if TTAL_AGENT_NAME is not set.
func captureSendTarget() string {
	agentName := os.Getenv("TTAL_AGENT_NAME")
	if agentName == "" {
		fmt.Fprintln(os.Stderr, "warning: TTAL_AGENT_NAME not set — no completion notification will be sent")
		return ""
	}
	jobID := os.Getenv("TTAL_JOB_ID")
	if jobID != "" {
		return jobID + ":" + agentName
	}
	return agentName
}

func runAgentList(_ *cobra.Command, _ []string) error {
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
		runtimes := []string{}
		if a.HasEiNative() {
			runtimes = append(runtimes, "ei-native")
		}
		if a.HasClaudeCode() {
			runtimes = append(runtimes, "claude-code")
		}
		rtStr := strings.Join(runtimes, ",")
		fmt.Printf("%s %-20s %-20s %s\n", emoji, a.Frontmatter.Name, rtStr, a.Frontmatter.Description)
	}
	return nil
}

func runAgentSync(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	targetDir := agentSyncFlags.target
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		targetDir = filepath.Join(home, ".claude", "agents")
	}

	result, err := agent.Sync(cfg.AgentsPaths, targetDir, agentSyncFlags.dryRun)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if len(result.Written) > 0 {
		fmt.Println("Written:")
		for _, name := range result.Written {
			fmt.Printf("  %s\n", name)
		}
	}

	if len(result.Skipped) > 0 {
		fmt.Println("Skipped (no claude-code block):")
		for _, name := range result.Skipped {
			fmt.Printf("  %s\n", name)
		}
	}

	if agentSyncFlags.dryRun {
		fmt.Println("\n(Dry run - no files written)")
	} else {
		fmt.Printf("\nSynced %d agents to %s\n", len(result.Written), targetDir)
	}

	return nil
}

// blockingEndpoint marshals req as JSON, POSTs to the daemon endpoint, reads
// the JSON response body, and decodes into T. Uses ctx for ctrl-c cancellation.
func blockingEndpoint[T any](ctx context.Context, endpoint string, req any) (*T, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://einai/"+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := newUnixClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("daemon unreachable (is 'ei daemon run' running?): %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result T
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// buildPrompt combines positional arguments and stdin into a single prompt.
func buildPrompt(args []string) string {
	var stdinContent string
	var positionalPrompt string

	// Read stdin if piped
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil {
			stdinContent = string(data)
		}
	}

	// Get positional prompt if provided
	if len(args) > 1 {
		positionalPrompt = args[1]
	}

	// Combine: stdin content + positional instruction
	if stdinContent != "" && positionalPrompt != "" {
		return stdinContent + "\n\n" + positionalPrompt
	}
	if stdinContent != "" {
		return stdinContent
	}
	if positionalPrompt != "" {
		return positionalPrompt
	}
	return ""
}
