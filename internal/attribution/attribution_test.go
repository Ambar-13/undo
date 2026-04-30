package attribution_test

import (
	"os"
	"testing"

	"github.com/Ambar13/undo/internal/attribution"
)

func TestDetectClaude(t *testing.T) {
	os.Setenv("CLAUDE_CODE_SESSION_ID", "session-abc")
	defer os.Unsetenv("CLAUDE_CODE_SESSION_ID")
	if attribution.Detect() != "claude" {
		t.Errorf("expected claude, got %q", attribution.Detect())
	}
}

func TestDetectCursor(t *testing.T) {
	os.Unsetenv("CLAUDE_CODE_SESSION_ID")
	os.Setenv("CURSOR_SESSION_ID", "session-xyz")
	defer os.Unsetenv("CURSOR_SESSION_ID")
	if attribution.Detect() != "cursor" {
		t.Errorf("expected cursor, got %q", attribution.Detect())
	}
}

func TestDetectAider(t *testing.T) {
	os.Unsetenv("CLAUDE_CODE_SESSION_ID")
	os.Unsetenv("CURSOR_SESSION_ID")
	os.Setenv("AIDER_SESSION_ID", "aider-123")
	defer os.Unsetenv("AIDER_SESSION_ID")
	if attribution.Detect() != "aider" {
		t.Errorf("expected aider, got %q", attribution.Detect())
	}
}

func TestDetectUser(t *testing.T) {
	os.Unsetenv("CLAUDE_CODE_SESSION_ID")
	os.Unsetenv("CURSOR_SESSION_ID")
	os.Unsetenv("AIDER_SESSION_ID")
	os.Unsetenv("COPILOT_SESSION_ID")
	if attribution.Detect() != "you" {
		t.Errorf("expected you, got %q", attribution.Detect())
	}
}
