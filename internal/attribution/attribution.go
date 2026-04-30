package attribution

import "os"

var agents = []struct {
	envKey string
	name   string
}{
	{"CLAUDE_CODE_SESSION_ID", "claude"},
	{"CURSOR_SESSION_ID", "cursor"},
	{"AIDER_SESSION_ID", "aider"},
	{"COPILOT_SESSION_ID", "copilot"},
}

// Detect returns the name of the AI agent running the current command,
// or "you" if no known agent environment variable is set.
func Detect() string {
	for _, a := range agents {
		if os.Getenv(a.envKey) != "" {
			return a.name
		}
	}
	return "you"
}
