package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
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

func buildFetchArgs(args []string) []string {
	webArgs := append([]string{"fetch"}, args...)
	return webArgs
}

func runFetch(cmd *cobra.Command, args []string) error {
	webCmd := exec.CommandContext(cmd.Context(), "web", buildFetchArgs(args)...)
	out, err := webCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("web fetch: %w\n%s", err, out)
	}
	fmt.Print(string(out))
	return nil
}
