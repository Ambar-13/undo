package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/Ambar-13/undo/internal/attribution"
	"github.com/Ambar-13/undo/internal/store"
)

var captureFileCmd = &cobra.Command{
	Use:    "capture-file <path>",
	Short:  "Capture a single file before it is modified (used by hooks and shims)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE:   runCaptureFile,
}

func init() {
	rootCmd.AddCommand(captureFileCmd)
}

func runCaptureFile(_ *cobra.Command, args []string) error {
	path := args[0]

	// Nothing to capture if the file doesn't exist yet
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	objStore := store.NewObjectStore(filepath.Join(home, storeRoot, "objects"))
	sessionsDir := filepath.Join(home, storeRoot, "sessions")
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		return err
	}
	journal := store.NewJournal(sessionsDir, sessionID())

	source := attribution.Detect()

	info, _ := os.Stat(path)
	var files []store.CapturedFile
	if info != nil && info.IsDir() {
		files = snapshotDir(objStore, path)
	} else {
		files = []store.CapturedFile{snapshotFile(objStore, path)}
	}

	entry := store.JournalEntry{
		ID:        uuid.New().String(),
		Timestamp: time.Now().UTC(),
		Command:   fmt.Sprintf("overwrite %s", filepath.Base(path)),
		Source:    source,
		Op:        store.OpOverwrite,
		SessionID: sessionID(),
		Files:     files,
	}

	if err := journal.Append(entry); err != nil {
		return err
	}

	for _, cf := range files {
		printIndicator(cf, source, 0)
	}
	return nil
}
