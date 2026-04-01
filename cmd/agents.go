package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/config"
)

var agentsSyncFlags struct {
	dryRun bool
	target string
}

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage agents",
}

var agentsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync agents to Claude Code format",
	RunE:  runAgentsSync,
}

func init() {
	agentsSyncCmd.Flags().BoolVar(&agentsSyncFlags.dryRun, "dry-run", false, "Show what would be written without writing")
	agentsSyncCmd.Flags().StringVar(&agentsSyncFlags.target, "target", "", "Target directory (default ~/.claude/agents)")
	agentsCmd.AddCommand(agentsSyncCmd)
	rootCmd.AddCommand(agentsCmd)
}

func runAgentsSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Set default target directory
	targetDir := agentsSyncFlags.target
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		targetDir = filepath.Join(home, ".claude", "agents")
	}

	result, err := agent.Sync(cfg.AgentsPaths, targetDir, agentsSyncFlags.dryRun)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	// Print results
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

	if agentsSyncFlags.dryRun {
		fmt.Println("\n(Dry run - no files written)")
	} else {
		fmt.Printf("\nSynced %d agents to %s\n", len(result.Written), targetDir)
	}

	return nil
}
