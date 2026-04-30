package store

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

type ObjectStore struct {
	root string
}

func NewObjectStore(root string) *ObjectStore {
	return &ObjectStore{root: root}
}

// validHash returns true iff h is a 64-character lowercase hex string.
func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func (s *ObjectStore) Put(content []byte) (string, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	dir := filepath.Join(s.root, hash[:2])
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, hash[2:])
	if _, err := os.Stat(path); err == nil {
		return hash, nil // already stored, dedup
	}
	tmp, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return hash, nil
}

func (s *ObjectStore) Get(hash string) ([]byte, error) {
	if !validHash(hash) {
		return nil, fmt.Errorf("invalid hash: %q", hash)
	}
	path := filepath.Join(s.root, hash[:2], hash[2:])
	return os.ReadFile(path)
}
