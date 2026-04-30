package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/Ambar-13/undo/internal/store"
)

var (
	purgeAll    bool
	purgeDryRun bool
)

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Remove old session journals and unreferenced objects from ~/.undo",
	Long: `Purge cleans up ~/.undo/ by removing session journals older than 24 hours
and any stored objects no longer referenced by remaining sessions.

By default, purge keeps the current session and any sessions from the last 24 hours.

Use --all to remove everything (including the current session's history).
Use --dry-run to preview what would be deleted without deleting anything.`,
	RunE: runPurge,
}

func init() {
	purgeCmd.Flags().BoolVar(&purgeAll, "all", false, "Remove all sessions and objects, including current session")
	purgeCmd.Flags().BoolVar(&purgeDryRun, "dry-run", false, "Show what would be deleted without deleting")
	rootCmd.AddCommand(purgeCmd)
}

func runPurge(_ *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	undoDir := filepath.Join(home, storeRoot)
	sessionsDir := filepath.Join(undoDir, "sessions")
	objectsDir := filepath.Join(undoDir, "objects")

	if purgeDryRun {
		fmt.Println("  dry run — nothing will be deleted")
		fmt.Println()
	}

	// Step 1: Find sessions to delete
	sessionEntries, err := os.ReadDir(sessionsDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading sessions: %w", err)
	}

	currentSID := sessionID()
	cutoff := time.Now().Add(-24 * time.Hour)

	var toDelete []string
	var toKeep []string

	for _, entry := range sessionEntries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(sessionsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		// Strip .journal suffix to get session ID
		sid := name
		if len(name) > 8 && name[len(name)-8:] == ".journal" {
			sid = name[:len(name)-8]
		}
		// Always keep current session unless --all
		isCurrent := sid == currentSID
		isOld := info.ModTime().Before(cutoff)

		if purgeAll || (isOld && !isCurrent) {
			toDelete = append(toDelete, path)
		} else {
			toKeep = append(toKeep, path)
		}
	}

	// Step 2: Collect referenced hashes from kept sessions
	referencedHashes := make(map[string]bool)
	for _, sessionPath := range toKeep {
		data, err := os.ReadFile(sessionPath)
		if err != nil {
			continue
		}
		j := &store.JournalReader{}
		entries := j.ParseBytes(data)
		for _, e := range entries {
			for _, f := range e.Files {
				if f.Hash != "" {
					referencedHashes[f.Hash] = true
				}
			}
		}
	}

	// Step 3: Find orphaned objects
	var orphanedObjects []string
	var orphanedBytes int64
	_ = filepath.WalkDir(objectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(objectsDir, path)
		if err != nil {
			return nil
		}
		hashParts := splitPath(rel)
		if len(hashParts) != 2 {
			return nil
		}
		hash := hashParts[0] + hashParts[1]
		if purgeAll || !referencedHashes[hash] {
			info, _ := d.Info()
			if info != nil {
				orphanedBytes += info.Size()
			}
			orphanedObjects = append(orphanedObjects, path)
		}
		return nil
	})

	// Step 4: Report
	sessionBytes := int64(0)
	for _, p := range toDelete {
		info, err := os.Stat(p)
		if err == nil {
			sessionBytes += info.Size()
		}
	}
	totalBytes := sessionBytes + orphanedBytes

	fmt.Printf("  sessions   %d to remove", len(toDelete))
	if len(toDelete) > 0 {
		fmt.Printf("  (%s)", humanBytes(sessionBytes))
	}
	fmt.Println()
	fmt.Printf("  objects    %d to remove  (%s)\n", len(orphanedObjects), humanBytes(orphanedBytes))
	fmt.Printf("  total      %s freed\n", humanBytes(totalBytes))

	if len(toDelete) == 0 && len(orphanedObjects) == 0 {
		fmt.Println()
		fmt.Println("  Nothing to purge.")
		return nil
	}

	if purgeDryRun {
		return nil
	}

	// Step 5: Delete
	for _, p := range toDelete {
		os.Remove(p)
	}
	for _, p := range orphanedObjects {
		os.Remove(p)
	}
	// Remove empty object subdirs
	_ = filepath.WalkDir(objectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == objectsDir {
			return nil
		}
		entries, _ := os.ReadDir(path)
		if len(entries) == 0 {
			os.Remove(path)
		}
		return nil
	})

	fmt.Println()
	fmt.Println("  ✓ purge complete")
	return nil
}

func splitPath(rel string) []string {
	// Split "AB/CDEF..." into ["AB", "CDEF..."]
	for i, c := range rel {
		if c == '/' || c == os.PathSeparator {
			return []string{rel[:i], rel[i+1:]}
		}
	}
	return []string{rel}
}

func humanBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}
