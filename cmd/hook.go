package cmd

import (
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Handle a Claude Code PreToolUse event from stdin (used by Claude Code integration)",
	Long: `Reads a Claude Code PreToolUse JSON payload from stdin and captures
any files that will be modified before the tool runs.

Install with: undo install --claude-code`,
	Hidden: true,
	RunE:   runHook,
}

func init() {
	rootCmd.AddCommand(hookCmd)
}

type hookPayload struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type fileToolInput struct {
	FilePath string `json:"file_path"`
}

type bashToolInput struct {
	Command string `json:"command"`
}

func runHook(_ *cobra.Command, _ []string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil || len(data) == 0 {
		return nil // never block Claude Code on hook errors
	}

	var payload hookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}

	switch payload.ToolName {
	case "Write", "Edit", "MultiEdit":
		var input fileToolInput
		if err := json.Unmarshal(payload.ToolInput, &input); err != nil {
			return nil
		}
		if input.FilePath != "" {
			// Capture the file before Claude modifies it
			return runCaptureFile(nil, []string{input.FilePath})
		}

	case "Bash":
		var input bashToolInput
		if err := json.Unmarshal(payload.ToolInput, &input); err != nil {
			return nil
		}
		if input.Command != "" {
			// Run through the normal shell classifier
			parts := strings.Fields(input.Command)
			if len(parts) > 0 {
				return runCapture(nil, parts)
			}
		}
	}

	return nil
}
