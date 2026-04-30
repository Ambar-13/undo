package store_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Ambar-13/undo/internal/store"
)

func TestJournalAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	j := store.NewJournal(dir, "sess-test")

	entry := store.JournalEntry{
		ID:        "e1",
		Timestamp: time.Now().UTC(),
		Command:   "rm foo.txt",
		Source:    "you",
		Op:        store.OpDelete,
		SessionID: "sess-test",
	}

	if err := j.Append(entry); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, err := j.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Command != "rm foo.txt" {
		t.Errorf("Command mismatch: %q", entries[0].Command)
	}
}

func TestJournalLastN(t *testing.T) {
	dir := t.TempDir()
	j := store.NewJournal(dir, "sess-lastn")

	for i := 0; i < 5; i++ {
		if err := j.Append(store.JournalEntry{
			ID:      fmt.Sprintf("e%d", i),
			Command: fmt.Sprintf("cmd%d", i),
		}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	last2, err := j.LastN(2)
	if err != nil {
		t.Fatalf("LastN: %v", err)
	}
	if len(last2) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(last2))
	}
	if last2[0].Command != "cmd3" || last2[1].Command != "cmd4" {
		t.Errorf("wrong entries: %q %q", last2[0].Command, last2[1].Command)
	}
}

func TestJournalEmptySession(t *testing.T) {
	dir := t.TempDir()
	j := store.NewJournal(dir, "sess-empty")
	entries, err := j.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll on empty: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(entries))
	}
}
