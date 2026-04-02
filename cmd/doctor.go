package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/logos"
)

var (
	doctorFix bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and configuration",
	Long: `Run diagnostics to verify einai is correctly configured and dependencies are available.

Checks performed:
  - Temenos daemon is running and reachable
  - ttal binary is on PATH
  - Config files exist
  - Agent paths are configured and contain agents
  - Einai daemon socket exists`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Create default config files if missing")
	rootCmd.AddCommand(doctorCmd)
}

type checkResult struct {
	pass   bool
	desc   string
	reason string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	checks := make([]checkResult, 0, 5)

	// Check 1: Temenos daemon running
	checks = append(checks, checkTemenos())

	// Check 2: ttal binary on PATH
	checks = append(checks, checkTtalBinary())

	// Check 3: Config files exist
	checks = append(checks, checkConfigFiles())

	// Check 4: Agent paths exist and contain agents
	checks = append(checks, checkAgentPaths())

	// Check 5: Einai daemon socket exists
	checks = append(checks, checkDaemonSocket())

	// Print results
	hasFailures := false
	for _, c := range checks {
		if c.pass {
			fmt.Printf("✓ %s\n", c.desc)
		} else {
			fmt.Printf("✗ %s: %s\n", c.desc, c.reason)
			hasFailures = true
		}
	}

	// Handle --fix flag
	if doctorFix {
		fixConfigFiles()
	}

	if hasFailures {
		return fmt.Errorf("one or more checks failed")
	}

	fmt.Println("\nAll checks passed!")
	return nil
}

func checkTemenos() checkResult {
	ctx := context.Background()
	tc, err := logos.NewClient("")
	if err != nil {
		return checkResult{
			pass:   false,
			desc:   "Temenos daemon running",
			reason: fmt.Sprintf("cannot connect: %v", err),
		}
	}

	// Check if it implements Health interface
	if hc, ok := tc.(interface{ Health(context.Context) error }); ok {
		if err := hc.Health(ctx); err != nil {
			return checkResult{
				pass:   false,
				desc:   "Temenos daemon running",
				reason: fmt.Sprintf("health check failed: %v", err),
			}
		}
	}

	return checkResult{pass: true, desc: "Temenos daemon running"}
}

func checkTtalBinary() checkResult {
	_, err := exec.LookPath("ttal")
	if err != nil {
		return checkResult{
			pass:   false,
			desc:   "ttal binary on PATH",
			reason: "ttal not found in PATH",
		}
	}
	return checkResult{pass: true, desc: "ttal binary on PATH"}
}

func checkConfigFiles() checkResult {
	configDir := config.DefaultConfigDir()
	configPath := filepath.Join(configDir, "config.toml")

	issues := []string{}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		issues = append(issues, "config.toml missing")
	}

	if len(issues) > 0 {
		return checkResult{
			pass:   false,
			desc:   "Config files exist",
			reason: strings.Join(issues, ", "),
		}
	}

	return checkResult{pass: true, desc: "Config files exist"}
}

func checkAgentPaths() checkResult {
	cfg, err := config.Load()
	if err != nil {
		return checkResult{
			pass:   false,
			desc:   "Agent paths configured",
			reason: fmt.Sprintf("failed to load config: %v", err),
		}
	}

	// Use default if AgentsPaths is empty
	paths := cfg.AgentsPaths
	if len(paths) == 0 {
		defaultPath := filepath.Join(config.DefaultConfigDir(), "agents")
		paths = []string{defaultPath}
	}

	hasValidPath := false
	for _, p := range paths {
		expanded := config.ExpandHome(p)
		if info, err := os.Stat(expanded); err == nil && info.IsDir() {
			// Check if path contains at least one .md file
			entries, err := os.ReadDir(expanded)
			if err != nil {
				return checkResult{
					pass:   false,
					desc:   "Agent paths configured with agents",
					reason: fmt.Sprintf("cannot read %s: %v", expanded, err),
				}
			}
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
					hasValidPath = true
					break
				}
			}
		}
	}

	if !hasValidPath {
		return checkResult{
			pass:   false,
			desc:   "Agent paths configured with agents",
			reason: "no agent paths contain .md files",
		}
	}

	return checkResult{pass: true, desc: "Agent paths configured with agents"}
}

func checkDaemonSocket() checkResult {
	socketPath := filepath.Join(config.DefaultDataDir(), "daemon.sock")
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return checkResult{
			pass:   false,
			desc:   "Einai daemon socket exists",
			reason: "socket not found (daemon may not be running)",
		}
	}
	return checkResult{pass: true, desc: "Einai daemon socket exists"}
}

func fixConfigFiles() {
	configDir := config.DefaultConfigDir()

	// Create config directory if needed
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		fmt.Printf("⚠ Failed to create config directory: %v\n", err)
		return
	}

	// Create minimal config.toml if missing
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := `# Einai configuration
model = "claude-sonnet-4-6"
max_steps = 100
max_tokens = 131072

# Paths to search for agent .md files
agents_paths = []

[rate_limit]
requests_per_minute = 0
concurrent_sessions = 0
`
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0o644); err != nil {
			fmt.Printf("⚠ Failed to create config.toml: %v\n", err)
		} else {
			fmt.Printf("✓ Created %s\n", configPath)
		}
	}

	// Create agents directory if missing
	agentsPath := filepath.Join(configDir, "agents")
	if err := os.MkdirAll(agentsPath, 0o755); err != nil {
		fmt.Printf("⚠ Failed to create agents directory: %v\n", err)
	} else {
		fmt.Printf("✓ Created %s\n", agentsPath)
	}
}
