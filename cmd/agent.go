package cmd

import (
	"fmt"
	"io"
	"os"
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
	Long: `Run a named agent using its frontmatter configuration (model, access level, system prompt).
The agent loop runs in the einai daemon via logos+temenos.

Use --task to load a taskwarrior task by ID (8-char hex or full UUID).
The task must exist and be in pending status.

Prompt can be piped via stdin, provided as argument, or both. When both stdin
and positional argument are provided, they are combined (stdin content + instruction).

Examples:
  ei agent run coder "implement the auth module"
  ei agent run coder --task abc12345
  ei agent run coder --task 12345678-1234-...
  cat plan.md | ei agent run coder
  cat plan.md | ei agent run coder "implement this plan"
  ei agent run pr-code-reviewer "review the current diff"`,
	Args:              cobra.RangeArgs(1, 2),
	RunE:              runAgent,
	ValidArgsFunction: agentNameCompletion,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agents",
	RunE:  runAgentList,
}

var agentFlags struct {
	env  []string
	task string
}

func init() {
	agentRunCmd.Flags().StringArrayVar(&agentFlags.env, "env", nil, "Extra env vars (KEY=VALUE)")
	agentRunCmd.Flags().StringVar(&agentFlags.task, "task", "", "Taskwarrior task ID (8-char hex or full UUID)")
	agentCmd.AddCommand(agentRunCmd)
	agentCmd.AddCommand(agentListCmd)
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

	var taskID *session.TaskID

	// Handle --task flag first
	if agentFlags.task != "" {
		tid := session.TaskID(agentFlags.task)
		if !tid.IsValid() {
			return fmt.Errorf("invalid task ID format: must be 8-char hex or full UUID")
		}
		// Validate task exists and is pending via taskwarrior
		if err := tid.ValidateWithTaskwarrior(); err != nil {
			return err
		}
		taskID = &tid
	}

	// Build prompt from positional args and/or stdin
	agentPrompt := buildPrompt(args)

	if agentPrompt == "" && taskID == nil {
		return fmt.Errorf("prompt or --task required — pass as argument, pipe via stdin, or use --task")
	}

	sandboxEnv := make(map[string]string)
	for _, kv := range agentFlags.env {
		k, v, _ := strings.Cut(kv, "=")
		sandboxEnv[k] = v
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}

	req := session.AgentRequest{
		Name:       name,
		Prompt:     agentPrompt,
		SandboxEnv: sandboxEnv,
		WorkingDir: cwd,
		TaskID:     taskID,
	}

	_, err = streamEndpoint(cmd.Context(), "agent/run", req)
	return err
}

// buildPrompt combines positional arguments and stdin into a single prompt.
// If stdin has content AND positional prompt exists: combine them (stdin + positional)
// If only positional: use positional
// If only stdin: use stdin
// If neither: return empty string
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
		fmt.Printf("%s %-20s %s\n", emoji, a.Frontmatter.Name, a.Frontmatter.Description)
	}
	return nil
}
