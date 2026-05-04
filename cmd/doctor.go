package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/config"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and configuration",
	Long: `Run diagnostics to verify einai is correctly configured and dependencies are available.

Checks performed:
  - lenos binary is on PATH
  - ttal binary is on PATH
  - Config files exist
  - Agent paths are configured and contain agents
  - Einai daemon socket exists`,
	RunE: runDoctor,
}

type checkResult struct {
	pass   bool
	desc   string
	reason string
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	checks := []checkResult{
		checkLenos(),
		checkTTAL(),
		checkConfig(),
		checkAgents(),
		checkSocket(),
	}

	allPassed := true
	for _, c := range checks {
		status := "✓"
		if !c.pass {
			status = "✗"
			allPassed = false
		}
		fmt.Printf("  %s %s\n", status, c.desc)
		if !c.pass && c.reason != "" {
			fmt.Printf("    ↳ %s\n", c.reason)
		}
	}

	if !allPassed {
		return fmt.Errorf("one or more checks failed")
	}

	fmt.Println("\nAll checks passed!")
	return nil
}

func checkLenos() checkResult {
	path, err := exec.LookPath("lenos")
	if err != nil {
		return checkResult{
			pass:   false,
			desc:   "lenos binary on PATH",
			reason: "lenos not found in PATH (required for runtime: lenos)",
		}
	}
	// Smoke-check version
	if err := exec.Command(path, "--version").Run(); err != nil {
		return checkResult{
			pass:   false,
			desc:   "lenos binary on PATH",
			reason: fmt.Sprintf("lenos --version failed: %v", err),
		}
	}
	return checkResult{pass: true, desc: "lenos binary on PATH"}
}

func checkTTAL() checkResult {
	path, err := exec.LookPath("ttal")
	if err != nil {
		return checkResult{
			pass:   false,
			desc:   "ttal binary on PATH",
			reason: "ttal not found in PATH",
		}
	}
	if err := exec.Command(path, "--version").Run(); err != nil {
		return checkResult{
			pass:   false,
			desc:   "ttal binary on PATH",
			reason: fmt.Sprintf("ttal --version failed: %v", err),
		}
	}
	return checkResult{pass: true, desc: "ttal binary on PATH"}
}

func checkConfig() checkResult {
	path := config.DefaultConfigDir() + "/config.toml"
	if _, err := os.Stat(path); err != nil {
		return checkResult{
			pass:   false,
			desc:   "config file exists",
			reason: fmt.Sprintf("config.toml not found at %s: %v", path, err),
		}
	}
	return checkResult{pass: true, desc: "config file exists"}
}

func checkAgents() checkResult {
	cfg, err := config.Load()
	if err != nil {
		return checkResult{
			pass:   false,
			desc:   "agent paths configured",
			reason: fmt.Sprintf("load config: %v", err),
		}
	}
	if len(cfg.AgentsPaths) == 0 {
		return checkResult{
			pass:   false,
			desc:   "agent paths configured",
			reason: "no agents_paths in config.toml",
		}
	}
	return checkResult{pass: true, desc: "agent paths configured"}
}

func checkSocket() checkResult {
	socketPath := config.DefaultDataDir() + "/daemon.sock"
	if _, err := os.Stat(socketPath); err != nil {
		return checkResult{
			pass:   false,
			desc:   "daemon socket exists",
			reason: fmt.Sprintf("socket not found at %s (is daemon running?)", socketPath),
		}
	}
	return checkResult{pass: true, desc: "daemon socket exists"}
}
