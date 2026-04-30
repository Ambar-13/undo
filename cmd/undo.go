package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Ambar-13/undo/internal/restore"
	"github.com/Ambar-13/undo/internal/store"
)

// restoreCmd is a named alias for the default root behaviour, useful for
// discoverability and scripting: "undo restore 3" is self-documenting.
var restoreCmd = &cobra.Command{
	Use:   "restore [N]",
	Short: "Undo the last N filesystem operations (default 1)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUndo,
}

func init() {
	rootCmd.AddCommand(restoreCmd)
}

func runUndo(_ *cobra.Command, args []string) error {
	n := 1
	if len(args) > 0 {
		_, _ = fmt.Sscanf(args[0], "%d", &n)
	}
	if n < 1 {
		n = 1
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	objStore := store.NewObjectStore(filepath.Join(home, storeRoot, "objects"))
	journal := store.NewJournal(filepath.Join(home, storeRoot, "sessions"), sessionID())

	entries, err := journal.LastN(n)
	if err != nil {
		return fmt.Errorf("reading journal: %w", err)
	}
	if len(entries) == 0 {
		// Check whether the shell hook is even installed; guide first-time users.
		scriptPath := filepath.Join(home, ".config", "undo", "shell", "undo."+detectShell())
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			fmt.Println("  Nothing to undo.  (shell hook not active — run: undo install)")
		} else {
			fmt.Println("  Nothing to undo.")
		}
		return nil
	}

	if len(entries) > 1 {
		fmt.Printf("  About to restore %d operations:\n\n", len(entries))
		for _, e := range entries {
			fmt.Printf("  · %s  [%s]\n", e.Command, e.Source)
		}
		fmt.Print("\n  Restore all? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("  Cancelled.")
			return nil
		}
	}

	// Show a heads-up for large restores so the user knows work is in progress.
	totalFiles := 0
	for _, e := range entries {
		totalFiles += len(e.Files)
	}
	if totalFiles > 20 {
		fmt.Printf("  restoring %d files...\n", totalFiles)
	}

	for i := len(entries) - 1; i >= 0; i-- {
		result := restore.Apply(entries[i], objStore)
		fmt.Printf("  ✓ restored  %s\n", entries[i].Command)
		for _, errMsg := range result.Errors {
			fmt.Fprintf(os.Stderr, "    ↳ %s\n", errMsg)
		}
	}
	return nil
}
