package store_test

import (
	"fmt"
	"os"
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

func TestJournalLastNMoreThanTotal(t *testing.T) {
	dir := t.TempDir()
	j := store.NewJournal(dir, "sess-overflow")
	_ = j.Append(store.JournalEntry{ID: "e1", Command: "rm a"})
	_ = j.Append(store.JournalEntry{ID: "e2", Command: "rm b"})

	entries, err := j.LastN(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2, got %d", len(entries))
	}
}

func TestJournalLastNZero(t *testing.T) {
	dir := t.TempDir()
	j := store.NewJournal(dir, "sess-zero")
	_ = j.Append(store.JournalEntry{ID: "e1", Command: "rm x"})

	entries, err := j.LastN(0)
	if err != nil {
		t.Fatal(err)
	}
	// n=0 returns no entries — nothing to undo
	if len(entries) != 0 {
		t.Errorf("LastN(0) should return 0 entries, got %d", len(entries))
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

func TestJournalSkipsCorruptedLines(t *testing.T) {
	dir := t.TempDir()
	j := store.NewJournal(dir, "sess-corrupt")

	good := store.JournalEntry{ID: "good", Command: "rm good.txt"}
	j.Append(good)

	// Manually inject a corrupted line
	import_path := dir + "/sess-corrupt.journal"
	f, _ := openAppend(import_path)
	_, _ = f.WriteString("this is not valid json\n")
	f.Close()

	j.Append(store.JournalEntry{ID: "also-good", Command: "rm also-good.txt"})

	entries, err := j.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries (corrupted line skipped), got %d", len(entries))
	}
}

func TestJournalPreservesFileInfo(t *testing.T) {
	dir := t.TempDir()
	j := store.NewJournal(dir, "sess-files")

	entry := store.JournalEntry{
		ID:      "e1",
		Command: "rm data.bin",
		Op:      store.OpDelete,
		Files: []store.CapturedFile{
			{Path: "/tmp/data.bin", Hash: "abc123def456" + fmt.Sprintf("%052d", 0), Captured: true, Mode: 0644},
			{Path: "/tmp/skip.bin", Captured: false, SkipReason: "too large"},
		},
	}
	j.Append(entry)

	entries, _ := j.ReadAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	files := entries[0].Files
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Captured != true || files[0].Mode != 0644 {
		t.Errorf("first file: %+v", files[0])
	}
	if files[1].Captured != false || files[1].SkipReason != "too large" {
		t.Errorf("second file: %+v", files[1])
	}
}

func TestJournalReaderParseBytes(t *testing.T) {
	dir := t.TempDir()
	j := store.NewJournal(dir, "parse-test")
	j.Append(store.JournalEntry{ID: "e1", Command: "rm a.txt"})
	j.Append(store.JournalEntry{ID: "e2", Command: "rm b.txt"})

	import_path := dir + "/parse-test.journal"
	data, err := readFile(import_path)
	if err != nil {
		t.Fatalf("reading journal file: %v", err)
	}

	r := &store.JournalReader{}
	entries := r.ParseBytes(data)
	if len(entries) != 2 {
		t.Errorf("expected 2, got %d", len(entries))
	}
}

func TestJournalReaderParseBytesWithGarbage(t *testing.T) {
	data := []byte(`{"id":"e1","command":"rm a.txt"}
not json at all
{"id":"e2","command":"rm b.txt"}
`)
	r := &store.JournalReader{}
	entries := r.ParseBytes(data)
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries, got %d", len(entries))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func openAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
