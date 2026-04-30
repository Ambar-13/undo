package store_test

import (
	"os"
	"testing"

	"github.com/Ambar-13/undo/internal/store"
)

func TestObjectStorePutGet(t *testing.T) {
	dir := t.TempDir()
	s := store.NewObjectStore(dir)

	content := []byte("hello undo")
	hash, err := s.Put(content)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("expected SHA-256 hex (64 chars), got %d", len(hash))
	}

	got, err := s.Get(hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q want %q", got, content)
	}
}

func TestObjectStorePutDeduplicates(t *testing.T) {
	dir := t.TempDir()
	s := store.NewObjectStore(dir)

	content := []byte("same content")
	hash1, err := s.Put(content)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	hash2, err := s.Put(content)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("expected same hash for same content")
	}
	// Verify only one file on disk under hash[:2] dir
	entries, err := os.ReadDir(dir + "/" + hash1[:2])
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 object, got %d", len(entries))
	}
}
