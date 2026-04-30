package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ambar-13/undo/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether undo is active in the current session",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(_ *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	journal := store.NewJournal(filepath.Join(home, storeRoot, "sessions"), sessionID())
	entries, err := journal.ReadAll()
	if err != nil {
		entries = nil
	}

	scriptPath := filepath.Join(home, ".config", "undo", "shell", "undo."+detectShell())
	shellHookInstalled := false
	if _, err := os.Stat(scriptPath); err == nil {
		shellHookInstalled = true
	}

	// Check Claude Code hook
	claudeSettingsPath := filepath.Join(home, ".claude", "settings.json")
	ccHookInstalled := false
	if data, err := os.ReadFile(claudeSettingsPath); err == nil {
		ccHookInstalled = strings.Contains(string(data), "undo hook")
	}

	fmt.Println("  undo  active")
	fmt.Println("  ──────────────────────────────────")
	fmt.Printf("  captured   %d operation(s) this session\n", len(entries))
	if shellHookInstalled {
		fmt.Printf("  shell hook installed\n")
	} else {
		fmt.Printf("  shell hook not installed — run: undo install\n")
	}
	if ccHookInstalled {
		fmt.Printf("  claude     hook installed\n")
	} else {
		fmt.Printf("  claude     hook not installed — run: undo install --claude-code\n")
	}
	return nil
}
