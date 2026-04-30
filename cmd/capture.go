package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/Ambar-13/undo/internal/attribution"
	"github.com/Ambar-13/undo/internal/classifier"
	"github.com/Ambar-13/undo/internal/store"
)

const (
	storeRoot    = ".undo"
	maxFileBytes = 50 * 1024 * 1024 // 50 MB per file
)

var captureCmd = &cobra.Command{
	Use:    "capture",
	Short:  "Capture filesystem state before a command runs (called by shell hook)",
	Hidden: true,
	Args:   cobra.MinimumNArgs(1),
	RunE:   runCapture,
}

func init() {
	rootCmd.AddCommand(captureCmd)
}

func runCapture(_ *cobra.Command, args []string) error {
	command := strings.Join(args, " ")
	op, paths := classifier.Classify(command)
	if op == store.OpUnknown {
		return nil
	}

	// git clean discovers paths dynamically — run a dry-run to find them before they're gone
	if len(paths) == 0 && classifier.IsGitClean(command) {
		paths = gitCleanPreview(command)
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
	entry := store.JournalEntry{
		ID:        uuid.New().String(),
		Timestamp: time.Now().UTC(),
		Command:   command,
		Source:    source,
		Op:        op,
		SessionID: sessionID(),
	}

	// Track what to print (collect first, print after journal append)
	type printJob struct {
		cf           store.CapturedFile
		isDirSummary bool
		dirName      string
		fileCount    int
	}
	var jobs []printJob

	for _, pattern := range paths {
		matches, _ := filepath.Glob(pattern)
		if len(matches) == 0 {
			matches = []string{pattern}
		}
		for _, path := range matches {
			info, statErr := os.Stat(path)
			if statErr == nil && info.IsDir() {
				dirFiles := snapshotDir(objStore, path)
				entry.Files = append(entry.Files, dirFiles...)
				captured := 0
				for _, cf := range dirFiles {
					if cf.Captured {
						captured++
					}
				}
				jobs = append(jobs, printJob{isDirSummary: true, dirName: filepath.Base(path), fileCount: captured})
				for _, cf := range dirFiles {
					if !cf.Captured {
						jobs = append(jobs, printJob{cf: cf})
					}
				}
			} else {
				cf := snapshotFile(objStore, path)
				entry.Files = append(entry.Files, cf)
				jobs = append(jobs, printJob{cf: cf})
			}
		}
	}

	if len(entry.Files) == 0 {
		entry.Files = []store.CapturedFile{{
			Path:       "(dynamic)",
			Captured:   false,
			SkipReason: "paths not statically known",
		}}
		jobs = append(jobs, printJob{cf: entry.Files[0]})
	}

	if err := journal.Append(entry); err != nil {
		return err
	}

	// Now we know the session count
	allEntries, _ := journal.ReadAll()
	sessionCount := len(allEntries)

	for _, job := range jobs {
		if job.isDirSummary {
			if job.fileCount > 0 {
				undoHint := "undo"
				if sessionCount > 1 {
					undoHint = fmt.Sprintf("undo %d to restore all", sessionCount)
				}
				agentTag := ""
				if source != "you" && source != "" {
					agentTag = fmt.Sprintf("  \033[2m·%s·\033[0m", source)
				}
				fmt.Fprintf(os.Stderr, "  \033[2m· captured  %s/ (%d files)  →  %s%s\033[0m\n",
					job.dirName, job.fileCount, undoHint, agentTag)
			}
		} else {
			printIndicator(job.cf, source, sessionCount)
		}
	}

	return nil
}

// snapshotFile captures a single file into the object store and returns a CapturedFile.
func snapshotFile(s *store.ObjectStore, path string) store.CapturedFile {
	info, err := os.Stat(path)
	if err != nil {
		return store.CapturedFile{Path: path, Captured: false, SkipReason: "not found"}
	}
	if info.Size() > maxFileBytes {
		return store.CapturedFile{
			Path:       path,
			Captured:   false,
			SkipReason: fmt.Sprintf("too large (%d MB)", info.Size()/1024/1024),
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return store.CapturedFile{Path: path, Captured: false, SkipReason: "read error"}
	}
	hash, err := s.Put(content)
	if err != nil {
		return store.CapturedFile{Path: path, Captured: false, SkipReason: "store error"}
	}
	return store.CapturedFile{Path: path, Hash: hash, Mode: uint32(info.Mode()), Captured: true}
}

// snapshotDir walks a directory and captures every file within it (up to 1000 files).
// It returns a slice of CapturedFile — one per file found, or one error entry if
// the directory is too large or a sentinel when the walk yields nothing.
func snapshotDir(s *store.ObjectStore, dir string) []store.CapturedFile {
	var files []store.CapturedFile
	var count int
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		count++
		if count > 1000 {
			files = append(files, store.CapturedFile{
				Path:       dir,
				Captured:   false,
				SkipReason: "directory too large (>1000 files)",
			})
			return filepath.SkipAll
		}
		files = append(files, snapshotFile(s, path))
		return nil
	})
	return files
}

func printIndicator(cf store.CapturedFile, source string, sessionCount int) {
	name := filepath.Base(cf.Path)
	if !cf.Captured {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ skipped  %s  (%s)\033[0m\n", name, cf.SkipReason)
		return
	}

	// Build undo hint based on session depth
	undoHint := "undo"
	if sessionCount > 1 {
		undoHint = fmt.Sprintf("undo %d to restore all", sessionCount)
	}

	// Show attribution inline when an AI agent ran the command
	agentTag := ""
	if source != "you" && source != "" {
		agentTag = fmt.Sprintf("  \033[2m·%s·\033[0m", source)
	}

	fmt.Fprintf(os.Stderr, "  \033[2m· captured  %s  →  %s%s\033[0m\n", name, undoHint, agentTag)
}

// gitCleanPreview runs `git clean --dry-run` with the same flags as the real
// command and returns the absolute paths of files that would be removed.
// This lets us capture them before the actual deletion happens.
func gitCleanPreview(command string) []string {
	// Don't preview if it's already a dry-run (no actual deletion will happen)
	if strings.Contains(command, "-n") || strings.Contains(command, "--dry-run") {
		return nil
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil
	}
	// Append --dry-run to the original command arguments
	dryArgs := append(parts[1:], "--dry-run")
	out, err := exec.Command(parts[0], dryArgs...).Output()
	if err != nil {
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		// git clean --dry-run outputs: "Would remove path/to/file"
		after, ok := strings.CutPrefix(strings.TrimSpace(line), "Would remove ")
		if !ok || after == "" {
			continue
		}
		// Strip trailing slash (directories are listed as "dir/")
		rel := strings.TrimSuffix(after, "/")
		paths = append(paths, filepath.Join(cwd, rel))
	}
	return paths
}

var _sessionID string

func sessionID() string {
	if _sessionID == "" {
		_sessionID = fmt.Sprintf("%d", os.Getppid())
	}
	return _sessionID
}
