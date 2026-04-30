package restore

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Ambar-13/undo/internal/store"
)

type Result struct {
	Entry  store.JournalEntry
	Errors []string
}

// Apply restores all captured files from a journal entry.
func Apply(entry store.JournalEntry, objStore *store.ObjectStore) Result {
	result := Result{Entry: entry}
	for _, f := range entry.Files {
		if !f.Captured || f.Hash == "" {
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: not captured, cannot restore", filepath.Base(f.Path)))
			continue
		}
		content, err := objStore.Get(f.Hash)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: object missing (%v)", filepath.Base(f.Path), err))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.Path), 0755); err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: mkdir failed (%v)", filepath.Base(f.Path), err))
			continue
		}
		mode := os.FileMode(f.Mode)
		if mode == 0 {
			mode = 0644
		}
		if err := os.WriteFile(f.Path, content, mode); err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: write failed (%v)", filepath.Base(f.Path), err))
		}
	}
	return result
}
