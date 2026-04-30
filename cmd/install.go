package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ambar13/undo/internal/shellscripts"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Add undo hooks to your shell rc file",
	RunE:  runInstall,
}

var installClaudeCode bool

func init() {
	installCmd.Flags().BoolVar(&installClaudeCode, "claude-code", false, "Also install hooks into Claude Code (~/.claude/settings.json)")
	rootCmd.AddCommand(installCmd)
}

func runInstall(_ *cobra.Command, _ []string) error {
	shell := detectShell()
	rcFile := rcFilePath(shell)
	if rcFile == "" {
		return fmt.Errorf("unsupported shell %q — manually source the appropriate integration file after running: undo install --print-script", shell)
	}

	// Extract embedded shell script to ~/.config/undo/shell/
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	shellDir := filepath.Join(home, ".config", "undo", "shell")
	if err := os.MkdirAll(shellDir, 0755); err != nil {
		return err
	}

	scriptName := "undo." + shell
	scriptPath := filepath.Join(shellDir, scriptName)

	scriptContent := shellScriptFor(shell)
	if scriptContent == nil {
		return fmt.Errorf("no embedded script for shell %q", shell)
	}

	if err := os.WriteFile(scriptPath, scriptContent, 0644); err != nil {
		return err
	}

	sourceLine := fmt.Sprintf("\n# undo — filesystem undo\nsource %q\n", scriptPath)

	content, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(content), "undo") {
		fmt.Printf("  undo is already configured in %s\n", rcFile)
		fmt.Printf("  Shell script updated at %s\n", scriptPath)
		// Still install Claude Code hook if requested, even when shell hook is already set up
		if installClaudeCode {
			if err := installClaudeCodeHook(); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: could not install Claude Code hook: %v\n", err)
			}
		}
		return nil
	}

	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(sourceLine); err != nil {
		return err
	}

	fmt.Printf("  Extracted shell integration to %s\n", scriptPath)
	fmt.Printf("  Added undo to %s\n", rcFile)
	fmt.Printf("  Restart your shell or run: source %s\n", rcFile)
	if shell == "bash" {
		fmt.Println()
		fmt.Println("  Note: bash integration requires bash-preexec.")
		fmt.Println("        Install it first: https://github.com/rcaloras/bash-preexec")
		fmt.Println("        Then add 'source ~/.bash-preexec.sh' above the undo line in .bashrc")
	}

	if installClaudeCode {
		if err := installClaudeCodeHook(); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not install Claude Code hook: %v\n", err)
		}
	}
	return nil
}

func installClaudeCodeHook() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}

	// Read existing settings or start fresh
	var settings map[string]interface{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	// Build the hook entry
	hookCommand := exe + " hook"
	hookEntry := map[string]interface{}{
		"matcher": "Write|Edit|MultiEdit|Bash",
		"hooks": []map[string]interface{}{
			{
				"type":    "command",
				"command": hookCommand,
			},
		},
	}

	// Check if already installed
	if hooks, ok := settings["hooks"].(map[string]interface{}); ok {
		if preToolUse, ok := hooks["PreToolUse"].([]interface{}); ok {
			for _, h := range preToolUse {
				if hm, ok := h.(map[string]interface{}); ok {
					if hm["matcher"] == "Write|Edit|MultiEdit|Bash" {
						fmt.Println("  Claude Code hook already installed")
						return nil
					}
				}
			}
		}
	}

	// Add or create hooks.PreToolUse
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
		settings["hooks"] = hooks
	}
	preToolUse, _ := hooks["PreToolUse"].([]interface{})
	hooks["PreToolUse"] = append(preToolUse, hookEntry)

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return err
	}

	fmt.Printf("  Installed Claude Code hook in %s\n", settingsPath)
	fmt.Println("  Claude Code will now capture Write/Edit/Bash operations automatically.")
	fmt.Println("  Restart Claude Code (quit + reopen) for the hook to take effect.")
	return nil
}

func shellScriptFor(shell string) []byte {
	switch shell {
	case "zsh":
		return shellscripts.Zsh
	case "bash":
		return shellscripts.Bash
	case "fish":
		return shellscripts.Fish
	default:
		return nil
	}
}

func detectShell() string {
	shell := os.Getenv("SHELL")
	switch {
	case strings.HasSuffix(shell, "zsh"):
		return "zsh"
	case strings.HasSuffix(shell, "bash"):
		return "bash"
	case strings.HasSuffix(shell, "fish"):
		return "fish"
	default:
		return filepath.Base(shell)
	}
}

func rcFilePath(shell string) string {
	home, _ := os.UserHomeDir()
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "bash":
		return filepath.Join(home, ".bashrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	default:
		return ""
	}
}
