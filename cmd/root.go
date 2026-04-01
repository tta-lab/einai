package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ei",
	Short: "Einai — native agent runtime daemon",
	Long:  "Einai is the native agent runtime for ttal. It owns the logos+temenos agent loop.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
