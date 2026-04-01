package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/sandbox"
)

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Manage sandbox configuration",
}

var sandboxSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync sandbox config to CC settings.json",
	Long:  "Regenerate the Claude Code settings.json sandbox section from ~/.config/einai/sandbox.toml. This controls which directories and network hosts agents may access.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		settingsPath, _ := cmd.Flags().GetString("settings-path")
		if settingsPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}
			settingsPath = filepath.Join(home, ".claude", "settings.json")
		}

		result, err := sandbox.SyncSandbox(settingsPath, dryRun)
		if err != nil {
			return fmt.Errorf("sync sandbox: %w", err)
		}

		if dryRun {
			fmt.Printf("dry-run: would write %d allow-write paths (%d git dirs)\n",
				len(result.AllowWritePaths), result.GitDirCount)
		} else {
			fmt.Printf("✓ sandbox synced: %d allow-write paths (%d git dirs)\n",
				len(result.AllowWritePaths), result.GitDirCount)
		}
		return nil
	},
}

func init() {
	sandboxSyncCmd.Flags().Bool("dry-run", false, "Show what would be written without writing")
	sandboxSyncCmd.Flags().String("settings-path", "", "Path to CC settings.json (default: ~/.claude/settings.json)")
	sandboxCmd.AddCommand(sandboxSyncCmd)
	rootCmd.AddCommand(sandboxCmd)
}
