package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/daemon"
)

// loadEnvFile loads environment variables from a .env file.
// Lines starting with # are treated as comments.
// Blank lines are skipped.
// Each key=value pair is split on the first '=' only.
// Variables already set in the environment are not overwritten.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip non-existent files
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split on first '=' only
		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}
		key := line[:idx]
		value := line[idx+1:]
		// Only set if not already set
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the einai daemon",
}

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the daemon in the foreground",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load .env files in order: ttal first, einai second (override)
		home, err := os.UserHomeDir()
		if err == nil {
			_ = loadEnvFile(home + "/.config/ttal/.env")
			_ = loadEnvFile(home + "/.config/einai/.env")
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		d := daemon.New(cfg)
		log.Printf("[einai] daemon starting")
		return d.Run(ctx)
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon health",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newUnixClient()
		resp, err := client.Get("http://einai/health")
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ daemon unreachable: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			fmt.Println("✓ daemon running")
		} else {
			fmt.Fprintf(os.Stderr, "✗ daemon returned %d\n", resp.StatusCode)
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonRunCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	rootCmd.AddCommand(daemonCmd)
}
