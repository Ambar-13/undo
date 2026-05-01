package restore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ambar-13/undo/internal/restore"
	"github.com/Ambar-13/undo/internal/store"
)

func newStore(t *testing.T) *store.ObjectStore {
	t.Helper()
	return store.NewObjectStore(filepath.Join(t.TempDir(), "objects"))
}

func TestRestoreFile(t *testing.T) {
	obj := newStore(t)
	content := []byte("original content")
	hash, _ := obj.Put(content)

	target := filepath.Join(t.TempDir(), "file.txt")
	result := restore.Apply(entry(target, hash, 0644), obj)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != string(content) {
		t.Fatalf("content mismatch: got %q err %v", got, err)
	}
}

func TestRestoreCreatesParentDir(t *testing.T) {
	obj := newStore(t)
	hash, _ := obj.Put([]byte("nested"))

	target := filepath.Join(t.TempDir(), "a", "b", "c", "file.txt")
	result := restore.Apply(entry(target, hash, 0644), obj)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("file not created in nested dir: %v", err)
	}
}

func TestRestorePreservesMode(t *testing.T) {
	obj := newStore(t)
	hash, _ := obj.Put([]byte("script"))

	target := filepath.Join(t.TempDir(), "run.sh")
	result := restore.Apply(entry(target, hash, 0755), obj)
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("mode: got %04o want 0755", info.Mode().Perm())
	}
}

func TestRestoreZeroModeDefaultsTo0644(t *testing.T) {
	obj := newStore(t)
	hash, _ := obj.Put([]byte("data"))

	target := filepath.Join(t.TempDir(), "file.txt")
	result := restore.Apply(entry(target, hash, 0), obj) // mode=0
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode: got %04o want 0644", info.Mode().Perm())
	}
}

func TestRestoreNotCapturedReportsError(t *testing.T) {
	obj := newStore(t)
	e := store.JournalEntry{
		Files: []store.CapturedFile{
			{Path: "/tmp/ghost.txt", Captured: false, SkipReason: "not found"},
		},
	}
	result := restore.Apply(e, obj)
	if len(result.Errors) == 0 {
		t.Fatal("expected error for not-captured file")
	}
}

func TestRestoreMissingObjectReportsError(t *testing.T) {
	obj := newStore(t)
	target := filepath.Join(t.TempDir(), "file.txt")
	e := store.JournalEntry{
		Files: []store.CapturedFile{
			{Path: target, Hash: strings.Repeat("a", 64), Captured: true},
		},
	}
	result := restore.Apply(e, obj)
	if len(result.Errors) == 0 {
		t.Fatal("expected error for missing object")
	}
}

func TestRestorePartialSuccess(t *testing.T) {
	obj := newStore(t)
	hash, _ := obj.Put([]byte("good"))
	dir := t.TempDir()

	e := store.JournalEntry{
		Files: []store.CapturedFile{
			{Path: filepath.Join(dir, "good.txt"), Hash: hash, Mode: 0644, Captured: true},
			{Path: filepath.Join(dir, "bad.txt"), Hash: strings.Repeat("b", 64), Captured: true}, // missing
			{Path: filepath.Join(dir, "skip.txt"), Captured: false, SkipReason: "too large"},
		},
	}
	result := restore.Apply(e, obj)
	if len(result.Errors) != 2 {
		t.Errorf("expected 2 errors (missing obj + not-captured), got %d: %v", len(result.Errors), result.Errors)
	}
	if _, err := os.Stat(filepath.Join(dir, "good.txt")); err != nil {
		t.Error("good.txt should have been restored")
	}
}

func TestRestoreOverwritesExistingFile(t *testing.T) {
	obj := newStore(t)
	original := []byte("original")
	hash, _ := obj.Put(original)

	target := filepath.Join(t.TempDir(), "file.txt")
	// Write something different first
	_ = os.WriteFile(target, []byte("modified by accident"), 0644)

	result := restore.Apply(entry(target, hash, 0644), obj)
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(original) {
		t.Errorf("expected original content, got %q", got)
	}
}

func TestRestoreEmptyFile(t *testing.T) {
	obj := newStore(t)
	hash, _ := obj.Put([]byte{})

	target := filepath.Join(t.TempDir(), "empty.txt")
	result := restore.Apply(entry(target, hash, 0644), obj)
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal("file not created")
	}
	if info.Size() != 0 {
		t.Errorf("expected empty file, got size %d", info.Size())
	}
}

func TestRestoreBinaryContent(t *testing.T) {
	obj := newStore(t)
	// Simulate binary file with null bytes
	content := []byte{0x00, 0xFF, 0x7F, 0x80, 0x01, 0x02}
	hash, _ := obj.Put(content)

	target := filepath.Join(t.TempDir(), "binary.bin")
	result := restore.Apply(entry(target, hash, 0644), obj)
	if len(result.Errors) != 0 {
		t.Fatalf("errors: %v", result.Errors)
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(content) {
		t.Errorf("binary content mismatch")
	}
}

func entry(path, hash string, mode os.FileMode) store.JournalEntry {
	return store.JournalEntry{
		Files: []store.CapturedFile{
			{Path: path, Hash: hash, Mode: uint32(mode), Captured: true},
		},
	}
}
