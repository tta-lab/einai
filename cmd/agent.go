package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
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

Prompt can be a positional argument or piped via stdin.

Examples:
  ei agent run coder "implement the auth module"
  cat plan.md | ei agent run coder
  ei agent run coder "implement X" --project myapp
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
	project    string
	repo       string
	maxSteps   int
	maxTokens  int
	env        []string
	workingDir string
}

func init() {
	agentRunCmd.Flags().StringVar(&agentFlags.project, "project", "", "Run in a registered project directory")
	agentRunCmd.Flags().StringVar(&agentFlags.repo, "repo", "", "Run in a cloned repo (read-only)")
	agentRunCmd.Flags().IntVar(&agentFlags.maxSteps, "max-steps", 0, "Maximum agent steps")
	agentRunCmd.Flags().IntVar(&agentFlags.maxTokens, "max-tokens", 0, "Maximum output tokens")
	agentRunCmd.Flags().StringArrayVar(&agentFlags.env, "env", nil, "Extra env vars (KEY=VALUE)")
	_ = agentRunCmd.RegisterFlagCompletionFunc("project", projectCompletion)
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

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}
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

	_, err = streamEndpoint(cmd.Context(), "agent/run", req, "agent run failed")
	return err
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
