package store_test

import (
	"os"
	"strings"
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
	hash1, _ := s.Put(content)
	hash2, _ := s.Put(content)
	if hash1 != hash2 {
		t.Errorf("expected same hash for same content")
	}
	entries, err := os.ReadDir(dir + "/" + hash1[:2])
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 object, got %d", len(entries))
	}
}

func TestObjectStoreGetInvalidHash(t *testing.T) {
	s := store.NewObjectStore(t.TempDir())

	cases := []string{
		"short",
		strings.Repeat("x", 64), // non-hex chars
		strings.Repeat("A", 64), // uppercase hex (invalid by validHash)
		"",
	}
	for _, h := range cases {
		if _, err := s.Get(h); err == nil {
			t.Errorf("Get(%q): expected error for invalid hash", h)
		}
	}
}

func TestObjectStoreGetMissingHash(t *testing.T) {
	s := store.NewObjectStore(t.TempDir())
	_, err := s.Get(strings.Repeat("a", 64))
	if err == nil {
		t.Error("expected error for non-existent object")
	}
}

func TestObjectStorePutEmptyContent(t *testing.T) {
	s := store.NewObjectStore(t.TempDir())
	hash, err := s.Put([]byte{})
	if err != nil {
		t.Fatalf("Put empty: %v", err)
	}
	got, err := s.Get(hash)
	if err != nil {
		t.Fatalf("Get empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d bytes", len(got))
	}
}

func TestObjectStorePutBinaryContent(t *testing.T) {
	s := store.NewObjectStore(t.TempDir())
	content := []byte{0x00, 0xFF, 0x7F, 0x80, 0x01, 0x02, 0xFE}
	hash, err := s.Put(content)
	if err != nil {
		t.Fatalf("Put binary: %v", err)
	}
	got, err := s.Get(hash)
	if err != nil {
		t.Fatalf("Get binary: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("binary content mismatch")
	}
}

func TestObjectStoreDifferentContentDifferentHash(t *testing.T) {
	s := store.NewObjectStore(t.TempDir())
	h1, _ := s.Put([]byte("foo"))
	h2, _ := s.Put([]byte("bar"))
	if h1 == h2 {
		t.Error("different content must produce different hashes")
	}
}
