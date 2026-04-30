package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

type Journal struct {
	path string
}

func NewJournal(dir, sessionID string) *Journal {
	return &Journal{path: filepath.Join(dir, sessionID+".journal")}
}

func (j *Journal) Append(entry JournalEntry) error {
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(entry)
}

func (j *Journal) ReadAll() ([]JournalEntry, error) {
	f, err := os.Open(j.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []JournalEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e JournalEntry
		if err := json.Unmarshal(line, &e); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, scanner.Err()
}

func (j *Journal) LastN(n int) ([]JournalEntry, error) {
	all, err := j.ReadAll()
	if err != nil || len(all) == 0 {
		return all, err
	}
	if n >= len(all) {
		return all, nil
	}
	return all[len(all)-n:], nil
}

// JournalReader parses raw journal bytes without needing a file path.
type JournalReader struct{}

func (r *JournalReader) ParseBytes(data []byte) []JournalEntry {
	var entries []JournalEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e JournalEntry
		if err := json.Unmarshal(line, &e); err == nil {
			entries = append(entries, e)
		}
	}
	return entries
}
