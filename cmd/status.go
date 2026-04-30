package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Ambar-13/undo/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show what undo is actively protecting",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(_ *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	journal := store.NewJournal(filepath.Join(home, storeRoot, "sessions"), sessionID())
	entries, err := journal.ReadAll()
	if err != nil {
		entries = nil
	}

	// ── Shell hook ───────────────────────────────────────────
	scriptPath := filepath.Join(home, ".config", "undo", "shell", "undo."+detectShell())
	shellHookInstalled := false
	if _, err := os.Stat(scriptPath); err == nil {
		shellHookInstalled = true
	}

	// ── Deep intercept library ───────────────────────────────
	libDir := filepath.Join(home, ".config", "undo", "lib")
	var libExt string
	if runtime.GOOS == "darwin" {
		libExt = "dylib"
	} else {
		libExt = "so"
	}
	libPath := filepath.Join(libDir, "libundo_intercept."+libExt)
	libExists := false
	if _, err := os.Stat(libPath); err == nil {
		libExists = true
	}

	// Check if LD_PRELOAD / DYLD_INSERT_LIBRARIES is active in the current process env
	ldPreloadActive := false
	envKey := "LD_PRELOAD"
	if runtime.GOOS == "darwin" {
		envKey = "DYLD_INSERT_LIBRARIES"
	}
	if val := os.Getenv(envKey); strings.Contains(val, "undo_intercept") {
		ldPreloadActive = true
	}

	// ── Claude Code settings ─────────────────────────────────
	claudeSettingsPath := filepath.Join(home, ".claude", "settings.json")
	ccHookInstalled := false
	ccEnvInstalled := false
	ccBinStale := false

	if data, err := os.ReadFile(claudeSettingsPath); err == nil {
		var settings map[string]interface{}
		if json.Unmarshal(data, &settings) == nil {
			// Check PreToolUse hook
			if hooks, ok := settings["hooks"].(map[string]interface{}); ok {
				if preToolUse, ok := hooks["PreToolUse"].([]interface{}); ok {
					for _, h := range preToolUse {
						if hm, ok := h.(map[string]interface{}); ok {
							if strings.Contains(fmt.Sprint(hm["matcher"]), "Bash") {
								ccHookInstalled = true
							}
						}
					}
				}
			}
			// Check env section for LD_PRELOAD / DYLD_INSERT_LIBRARIES
			if envSection, ok := settings["env"].(map[string]interface{}); ok {
				if val, ok := envSection[envKey].(string); ok && strings.Contains(val, "undo_intercept") {
					ccEnvInstalled = true
				}
				// Check if UNDO_BIN in settings still points to a real exe
				if undoBin, ok := envSection["UNDO_BIN"].(string); ok && undoBin != "" {
					if _, err := os.Stat(undoBin); os.IsNotExist(err) {
						ccBinStale = true
					}
				}
			}
		}
	}

	// ── Print ────────────────────────────────────────────────
	anyCoverage := shellHookInstalled || ccHookInstalled
	if anyCoverage {
		fmt.Println("  undo  active")
	} else {
		fmt.Println("  undo  not yet set up — run: undo install")
	}
	fmt.Println("  ──────────────────────────────────────────────────────")
	fmt.Printf("  captured   %d operation(s) this session\n", len(entries))
	fmt.Println()

	// Shell hook
	if shellHookInstalled {
		fmt.Println("  shell hook       ✓ installed  (rm, mv, overwrites in interactive shell)")
	} else {
		fmt.Println("  shell hook       ✗ not installed  →  run: undo install")
	}

	// Deep intercept (LD_PRELOAD library)
	if libExists && ldPreloadActive {
		fmt.Printf("  deep intercept   ✓ active  (subshells, make, subprocess, C programs)\n")
	} else if libExists && !ldPreloadActive {
		fmt.Printf("  deep intercept   ✓ compiled  (restart your shell to activate)\n")
	} else {
		fmt.Println("  deep intercept   ✗ not compiled  →  run: undo install")
	}

	fmt.Println()

	// Claude Code coverage
	if ccHookInstalled && ccEnvInstalled {
		fmt.Println("  claude hook      ✓ installed  (PreToolUse: Write/Edit/Bash)")
		fmt.Println("  claude intercept ✓ installed  (subshells inside Claude Code's bash)")
	} else if ccHookInstalled && !ccEnvInstalled {
		fmt.Println("  claude hook      ✓ installed  (PreToolUse: Write/Edit/Bash)")
		fmt.Println("  claude intercept ✗ not installed  →  run: undo install --claude-code")
	} else {
		fmt.Println("  claude hook      ✗ not installed  →  run: undo install --claude-code")
		fmt.Println("  claude intercept ✗ not installed  →  run: undo install --claude-code")
	}

	if ccBinStale {
		fmt.Println()
		fmt.Println("  ⚠  UNDO_BIN in ~/.claude/settings.json points to a missing binary.")
		fmt.Println("     Re-run: undo install --claude-code")
	}

	if !libExists {
		fmt.Println()
		fmt.Println("  Tip: install a C compiler (cc) to enable deep intercept, then re-run: undo install")
	}

	return nil
}
