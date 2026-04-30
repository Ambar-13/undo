package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/Ambar-13/undo/internal/store"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show captured filesystem operations in this session",
	RunE:  runHistory,
}

func init() {
	rootCmd.AddCommand(historyCmd)
}

func runHistory(_ *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	journal := store.NewJournal(filepath.Join(home, storeRoot, "sessions"), sessionID())

	entries, err := journal.ReadAll()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("  No captured operations this session.")
		return nil
	}

	hasAgents := false
	for _, e := range entries {
		if e.Source != "you" && e.Source != "" {
			hasAgents = true
			break
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if hasAgents {
		fmt.Fprintln(w, "  TIME\tBY\tACTION\tCOMMAND")
	} else {
		fmt.Fprintln(w, "  TIME\tACTION\tCOMMAND")
	}
	for _, e := range entries {
		t := e.Timestamp.Local().Format(time.Kitchen)
		action := opToAction(e.Op)
		cmd := truncateStr(e.Command, 80)
		restorability := restorabilityTag(e.Files)
		if hasAgents {
			by := e.Source
			if e.Source != "you" {
				by = "·" + e.Source + "·"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s%s\n", t, by, action, cmd, restorability)
		} else {
			fmt.Fprintf(w, "  %s\t%s\t%s%s\n", t, action, cmd, restorability)
		}
	}
	return w.Flush()
}

// restorabilityTag returns an inline suffix for history rows to show whether
// the entry can actually be restored. Returns "  ⚠ not captured" when all
// files were skipped, empty string when at least one file was captured.
func restorabilityTag(files []store.CapturedFile) string {
	if len(files) == 0 {
		return ""
	}
	for _, f := range files {
		if f.Captured {
			return ""
		}
	}
	// All files have Captured: false — show a note
	reason := files[0].SkipReason
	if reason == "" {
		reason = "not captured"
	}
	return fmt.Sprintf("  \033[33m⚠ %s\033[0m", reason)
}

func opToAction(op store.OpType) string {
	switch op {
	case store.OpDelete:
		return "removed"
	case store.OpMove:
		return "moved"
	case store.OpOverwrite:
		return "overwrote"
	default:
		return string(op)
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
