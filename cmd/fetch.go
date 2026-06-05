package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/agent"
	"github.com/tta-lab/einai/internal/config"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch <url> [web fetch flags]",
	Short: "Fetch a web page with organon web",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runFetch,
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}

func buildFetchArgs(target, model string) []string {
	return []string{
		"run",
		"--agent",
		"webdiver",
		"--readonly",
		"-m",
		model,
		"--",
		"Fetch and analyze " + target,
	}
}

func runFetch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	lenosCmd := exec.CommandContext(cmd.Context(), "lenos", buildFetchArgs(args[0], cfg.AgentModel())...)
	agentsDir, err := agent.WriteEmbeddedDir()
	if err != nil {
		return err
	}
	lenosCmd.Env = append(os.Environ(), "LENOS_AGENTS_DIR="+agentsDir)
	out, err := lenosCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lenos webdiver: %w\n%s", err, out)
	}
	return renderResult(string(out))
}
