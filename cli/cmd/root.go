package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sflowg",
	Short: "SFlowG - Workflow execution engine",
	Long: `SFlowG is a workflow execution engine that processes YAML-defined flows.

The CLI tool helps you build deployable binaries with selected plugins.`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(buildCmd)
}
