package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Ambar13/undo/internal/store"
)

func TestJournalEntryRoundTrip(t *testing.T) {
	entry := store.JournalEntry{
		ID:        "abc123",
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Command:   "rm -rf dist/",
		Source:    "claude",
		Op:        store.OpDelete,
		SessionID: "sess-1",
		Files: []store.CapturedFile{
			{Path: "/tmp/test.txt", Hash: "deadbeef", Captured: true},
		},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got store.JournalEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Command != entry.Command {
		t.Errorf("Command mismatch: got %q want %q", got.Command, entry.Command)
	}
	if got.Source != entry.Source {
		t.Errorf("Source mismatch: got %q want %q", got.Source, entry.Source)
	}
	if len(got.Files) != 1 || got.Files[0].Hash != "deadbeef" {
		t.Errorf("Files mismatch: %+v", got.Files)
	}
}
