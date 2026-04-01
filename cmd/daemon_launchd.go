package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	launchdLabel   = "io.tta.einai.daemon"
	launchdPlist   = "io.tta.einai.daemon.plist"
	launchdLogFile = "daemon.log"
)

const goosDarwin = "darwin"

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return home + "/Library/LaunchAgents/" + launchdPlist
}

func launchdLogPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.einai/" + launchdLogFile
}

func generatePlist(binaryPath string) string {
	logPath := launchdLogPath()
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, launchdLabel, binaryPath, logPath, logPath)
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the daemon as a launchd service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != goosDarwin {
			fmt.Println("launchd is only supported on macOS")
			return nil
		}

		binaryPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}

		plistPath := launchdPlistPath()
		plistContent := generatePlist(binaryPath)

		if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
			return fmt.Errorf("write plist: %w", err)
		}

		launchctlCmd := exec.Command("launchctl", "load", "-w", plistPath)
		if output, err := launchctlCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl load: %w (output: %s)", err, string(output))
		}

		fmt.Printf("✓ daemon installed: %s\n", plistPath)
		return nil
	},
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the daemon launchd service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != goosDarwin {
			fmt.Println("launchd is only supported on macOS")
			return nil
		}

		plistPath := launchdPlistPath()

		// Unload first
		launchctlCmd := exec.Command("launchctl", "unload", plistPath)
		launchctlCmd.Run() // ignore error - service might not be loaded

		// Remove plist file
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove plist: %w", err)
		}

		fmt.Printf("✓ daemon uninstalled: %s\n", plistPath)
		return nil
	},
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon via launchd",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != goosDarwin {
			fmt.Println("launchd is only supported on macOS")
			return nil
		}

		uid := os.Getuid()
		launchctlCmd := exec.Command("launchctl", "kickstart", "-k", fmt.Sprintf("gui/%d/%s", uid, launchdLabel))
		if output, err := launchctlCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl kickstart: %w (output: %s)", err, string(output))
		}

		fmt.Println("✓ daemon started")
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon via launchd",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != goosDarwin {
			fmt.Println("launchd is only supported on macOS")
			return nil
		}

		uid := os.Getuid()
		launchctlCmd := exec.Command("launchctl", "kill", "TERM", fmt.Sprintf("gui/%d/%s", uid, launchdLabel))
		if output, err := launchctlCmd.CombinedOutput(); err != nil {
			// Ignore errors if process isn't running
			if !strings.Contains(string(output), "No such process") {
				return fmt.Errorf("launchctl kill: %w (output: %s)", err, string(output))
			}
		}

		fmt.Println("✓ daemon stopped")
		return nil
	},
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the daemon via launchd (stop then start)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != goosDarwin {
			fmt.Println("launchd is only supported on macOS")
			return nil
		}

		// Stop
		uid := os.Getuid()
		launchctlKill := exec.Command("launchctl", "kill", "TERM", fmt.Sprintf("gui/%d/%s", uid, launchdLabel))
		launchctlKill.Run() // ignore error

		// Start
		launchctlKick := exec.Command("launchctl", "kickstart", "-k", fmt.Sprintf("gui/%d/%s", uid, launchdLabel))
		if output, err := launchctlKick.CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl kickstart: %w (output: %s)", err, string(output))
		}

		fmt.Println("✓ daemon restarted")
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonInstallCmd)
	daemonCmd.AddCommand(daemonUninstallCmd)
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
}
