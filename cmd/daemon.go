package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/config"
	"github.com/tta-lab/einai/internal/daemon"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the einai daemon",
}

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the daemon in the foreground",
	Long: "Start the einai daemon in the foreground. The daemon listens on a " +
		"unix socket and handles agent session requests from the CLI.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load .env files in order: ttal first, einai second.
		// godotenv.Load preserves OS env precedence (does not overwrite existing vars).
		home, err := os.UserHomeDir()
		if err == nil {
			for _, envPath := range []string{
				filepath.Join(home, ".config/ttal/.env"),
				filepath.Join(home, ".config/einai/.env"),
			} {
				if loadErr := godotenv.Load(envPath); loadErr != nil && !os.IsNotExist(loadErr) {
					log.Printf("[einai] warning: failed to load %s: %v", envPath, loadErr)
				}
			}
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
