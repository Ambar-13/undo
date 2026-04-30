package store

import "time"

type OpType string

const (
	OpDelete    OpType = "delete"
	OpMove      OpType = "move"
	OpOverwrite OpType = "overwrite"
	OpUnknown   OpType = "unknown"
)

type CapturedFile struct {
	Path       string `json:"path"`
	Hash       string `json:"hash,omitempty"`
	Mode       uint32 `json:"mode,omitempty"`
	Captured   bool   `json:"captured"`
	SkipReason string `json:"skip_reason,omitempty"`
}

type JournalEntry struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Command   string         `json:"command"`
	Source    string         `json:"source"`
	Op        OpType         `json:"op"`
	SessionID string         `json:"session_id"`
	Files     []CapturedFile `json:"files"`
}
