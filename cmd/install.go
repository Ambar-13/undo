package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Ambar-13/undo/internal/intercept"
	"github.com/Ambar-13/undo/internal/shellscripts"
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

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Extract embedded shell script to ~/.config/undo/shell/
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

	// Compile the intercept library (best-effort — degrade gracefully if cc absent)
	libPath, libErr := compileInterceptLib(home)
	if libErr != nil {
		fmt.Fprintf(os.Stderr, "  note: deep intercept unavailable (%v)\n", libErr)
		fmt.Fprintf(os.Stderr, "        subshells and subprocess calls won't be captured\n")
		fmt.Fprintf(os.Stderr, "        install a C compiler (cc) and re-run: undo install\n")
	} else {
		fmt.Printf("  Compiled intercept library → %s\n", libPath)
		fmt.Println("  Deep intercept covers: subshells, make, subprocess calls, C programs.")
	}

	sourceLine := fmt.Sprintf("\n# undo — filesystem undo\nsource %q\n", scriptPath)

	content, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(content), "undo") {
		fmt.Printf("  undo is already configured in %s\n", rcFile)
		fmt.Printf("  Shell script updated at %s\n", scriptPath)
		if installClaudeCode {
			if err := installClaudeCodeHook(libPath); err != nil {
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
		if err := installClaudeCodeHook(libPath); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not install Claude Code hook: %v\n", err)
		}
	}
	return nil
}

// compileInterceptLib compiles the C intercept library into ~/.config/undo/lib/.
// Returns the path to the compiled library, or an error if cc is unavailable.
func compileInterceptLib(home string) (string, error) {
	libDir := filepath.Join(home, ".config", "undo", "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		return "", err
	}

	// Write C source to a temp file
	srcFile, err := os.CreateTemp("", "undo_intercept_*.c")
	if err != nil {
		return "", err
	}
	defer os.Remove(srcFile.Name())

	if _, err := srcFile.Write(intercept.Source); err != nil {
		srcFile.Close()
		return "", err
	}
	srcFile.Close()

	// Output library path
	var libName string
	var compileArgs []string
	if runtime.GOOS == "darwin" {
		libName = "libundo_intercept.dylib"
		compileArgs = []string{"-dynamiclib", "-o"}
	} else {
		libName = "libundo_intercept.so"
		compileArgs = []string{"-shared", "-fPIC", "-ldl", "-o"}
	}
	libPath := filepath.Join(libDir, libName)
	compileArgs = append(compileArgs, libPath, srcFile.Name())

	out, err := exec.Command("cc", compileArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cc: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return libPath, nil
}

func installClaudeCodeHook(libPath string) error {
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
		_ = json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	// Add PreToolUse hook for Write/Edit/Bash operations
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

	alreadyHooked := false
	if hooks, ok := settings["hooks"].(map[string]interface{}); ok {
		if preToolUse, ok := hooks["PreToolUse"].([]interface{}); ok {
			for _, h := range preToolUse {
				if hm, ok := h.(map[string]interface{}); ok {
					if hm["matcher"] == "Write|Edit|MultiEdit|Bash" {
						alreadyHooked = true
						break
					}
				}
			}
		}
	}

	if !alreadyHooked {
		hooks, ok := settings["hooks"].(map[string]interface{})
		if !ok {
			hooks = make(map[string]interface{})
			settings["hooks"] = hooks
		}
		preToolUse, _ := hooks["PreToolUse"].([]interface{})
		hooks["PreToolUse"] = append(preToolUse, hookEntry)
		fmt.Printf("  Installed Claude Code hook in %s\n", settingsPath)
		fmt.Println("  Restart Claude Code (quit + reopen) for the hook to take effect.")
	} else {
		fmt.Println("  Claude Code hook already installed")
	}

	// Inject LD_PRELOAD / DYLD_INSERT_LIBRARIES into Claude Code's bash environment.
	// This covers all Bash tool subshells within Claude Code, not just top-level commands.
	if libPath != "" {
		envKey := "LD_PRELOAD"
		if runtime.GOOS == "darwin" {
			envKey = "DYLD_INSERT_LIBRARIES"
		}

		envSection, ok := settings["env"].(map[string]interface{})
		if !ok {
			envSection = make(map[string]interface{})
			settings["env"] = envSection
		}

		existing, _ := envSection[envKey].(string)
		if !strings.Contains(existing, libPath) {
			if existing != "" {
				envSection[envKey] = libPath + ":" + existing
			} else {
				envSection[envKey] = libPath
			}
			fmt.Printf("  Set %s in Claude Code env → %s\n", envKey, libPath)
		}

		// Always export the undo binary path so the intercept lib can find it
		if _, ok := envSection["UNDO_BIN"]; !ok {
			envSection["UNDO_BIN"] = exe
		}
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, data, 0644)
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
