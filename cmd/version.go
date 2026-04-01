package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var buildVersion, buildDate, buildGoVersion string

func SetBuildInfo(v, d, g string) {
	buildVersion = v
	buildDate = d
	buildGoVersion = g
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ei version %s\n", buildVersion)
		fmt.Printf("build date: %s\n", buildDate)
		fmt.Printf("go version: %s\n", buildGoVersion)
		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
