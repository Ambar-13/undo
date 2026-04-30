package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "undo [N]",
	Short: "Undo the last N filesystem operations (default 1)",
	Long: `Filesystem undo for terminal commands and AI agents.

Running 'undo' with no subcommand restores the last captured operation.
Run 'undo N' to restore the last N operations (e.g. 'undo 3').`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUndo,
}

func Execute() {
	// Hide completion from top-level help — power users can still access it
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
