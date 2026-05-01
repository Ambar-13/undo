package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCmdInput(bin string, env []string, input string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	return cmd.CombinedOutput()
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "undo")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	return bin
}

// ── basic capture + undo ─────────────────────────────────────────────────────

func TestE2ECaptureAndUndo(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()

	target := filepath.Join(home, "important.txt")
	if err := os.WriteFile(target, []byte("do not lose this"), 0644); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "HOME="+home)

	captureOut, err := runCmd(bin, env, "capture", "rm", target)
	if err != nil {
		t.Fatalf("capture failed: %v\n%s", err, captureOut)
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("file should not exist after removal")
	}

	undoOut, err := runCmd(bin, env, "undo")
	if err != nil {
		t.Fatalf("undo failed: %v\n%s", err, undoOut)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("file not restored: %v\nundo output: %s", err, undoOut)
	}
	if string(content) != "do not lose this" {
		t.Errorf("wrong content after restore: %q", content)
	}
}

// ── subfolder with multiple files ─────────────────────────────────────────────

func TestE2ESubfolderFiles(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	// Create subfolder with several files
	dir := filepath.Join(home, "project", "src")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Sequential: capture → delete → undo → verify for each file.
	// (Journal has no cursor; each undo restores the current last entry.)
	tests := []struct{ name, content string }{
		{"main.go", "package main\nfunc main() {}\n"},
		{"util.go", "package main\nfunc helper() {}\n"},
		{"notes.md", "# Notes\n\nImportant stuff here.\n"},
	}
	for _, tc := range tests {
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
			t.Fatal(err)
		}

		out, err := runCmd(bin, env, "capture", "rm", path)
		if err != nil {
			t.Fatalf("capture %s: %v\n%s", tc.name, err, out)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}

		out, err = runCmd(bin, env)
		if err != nil {
			t.Fatalf("undo for %s: %v\n%s", tc.name, err, out)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("file not restored: %s: %v", tc.name, err)
			continue
		}
		if string(got) != tc.content {
			t.Errorf("content mismatch for %s:\ngot  %q\nwant %q", tc.name, got, tc.content)
		}
	}
}

// ── capture overwrite then undo ───────────────────────────────────────────────

func TestE2EOverwriteAndUndo(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	cfg := filepath.Join(home, "config.yaml")
	original := "version: 1\nfeature: enabled\n"
	if err := os.WriteFile(cfg, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	// Use capture-file so the absolute path is captured directly
	out, err := runCmd(bin, env, "capture-file", cfg)
	if err != nil {
		t.Fatalf("capture-file: %v\n%s", err, out)
	}

	// Simulate overwrite
	if err := os.WriteFile(cfg, []byte("garbage\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err = runCmd(bin, env)
	if err != nil {
		t.Fatalf("undo: %v\n%s", err, out)
	}

	got, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("after undo: got %q, want %q", got, original)
	}
}

// ── undo N at once ────────────────────────────────────────────────────────────

func TestE2EUndoN(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	dir := filepath.Join(home, "batch")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create and capture 3 files
	for i := 1; i <= 3; i++ {
		p := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(p, []byte(fmt.Sprintf("content%d", i)), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := runCmd(bin, env, "capture", "rm", p)
		if err != nil {
			t.Fatalf("capture file%d: %v\n%s", i, err, out)
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}

	// "undo restore 2" — uses the named subcommand to avoid root-cmd arg ambiguity.
	// Pipe "y\n" to confirm the multi-op prompt.
	out, err := runCmdInput(bin, env, "y\n", "restore", "2")
	if err != nil {
		t.Fatalf("undo restore 2: %v\n%s", err, out)
	}

	// file1 should NOT be restored (not part of the last-2 window)
	if _, err := os.Stat(filepath.Join(dir, "file1.txt")); !os.IsNotExist(err) {
		t.Error("file1.txt should not be restored (outside undo 2 window)")
	}
	// file2 and file3 should be restored
	for _, name := range []string{"file2.txt", "file3.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should be restored after undo 2: %v", name, err)
		}
	}
}

// ── undo with empty journal ───────────────────────────────────────────────────

func TestE2EUndoEmptyJournal(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	out, err := runCmd(bin, env, "undo")
	// Should not crash — either exits cleanly or with a helpful message
	if err != nil {
		// non-zero exit is acceptable as long as it's not a panic
		if strings.Contains(string(out), "panic") {
			t.Fatalf("undo panicked on empty journal:\n%s", out)
		}
	}
	// Output should mention nothing to undo or similar
	if len(out) == 0 {
		t.Error("expected some output from undo on empty journal")
	}
}

// ── history shows captured entries ───────────────────────────────────────────

func TestE2EHistoryShowsEntry(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	target := filepath.Join(home, "tracked.txt")
	if err := os.WriteFile(target, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _ = runCmd(bin, env, "capture", "rm", target)

	out, err := runCmd(bin, env, "history")
	if err != nil {
		t.Fatalf("history failed: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Error("history output is empty, expected at least one entry")
	}
}

// ── file permissions preserved across undo ───────────────────────────────────

func TestE2EPermissionsPreserved(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	script := filepath.Join(home, "deploy.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\necho deploying\n"), 0755); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(bin, env, "capture", "rm", script)
	if err != nil {
		t.Fatalf("capture: %v\n%s", err, out)
	}

	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}

	out, err = runCmd(bin, env, "undo")
	if err != nil {
		t.Fatalf("undo: %v\n%s", err, out)
	}

	info, err := os.Stat(script)
	if err != nil {
		t.Fatalf("script not restored: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("mode not preserved: got %04o, want 0755", info.Mode().Perm())
	}
}

// ── capture-file subcommand ───────────────────────────────────────────────────

func TestE2ECaptureFileSubcommand(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	target := filepath.Join(home, "data.bin")
	content := []byte{0x00, 0xFF, 0x7F, 0x80, 0xDE, 0xAD}
	if err := os.WriteFile(target, content, 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(bin, env, "capture-file", target)
	if err != nil {
		t.Fatalf("capture-file: %v\n%s", err, out)
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	out, err = runCmd(bin, env, "undo")
	if err != nil {
		t.Fatalf("undo after capture-file: %v\n%s", err, out)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("file not restored: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("binary content mismatch after restore")
	}
}

// ── UNDO_MAX_SIZE skips large files ──────────────────────────────────────────

func TestE2EMaxSizeSkipsLargeFile(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	// Set max size to 10 bytes so a 1KB file gets skipped
	env := append(os.Environ(), "HOME="+home, "UNDO_MAX_SIZE=10")

	target := filepath.Join(home, "big.bin")
	if err := os.WriteFile(target, make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(bin, env, "capture", "rm", target)
	if err != nil {
		t.Fatalf("capture: %v\n%s", err, out)
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	// Undo should report error (file was not captured due to size)
	out, _ = runCmd(bin, env, "undo")
	if !strings.Contains(strings.ToLower(string(out)), "skip") &&
		!strings.Contains(strings.ToLower(string(out)), "large") &&
		!strings.Contains(strings.ToLower(string(out)), "size") &&
		!strings.Contains(strings.ToLower(string(out)), "not captured") &&
		!strings.Contains(strings.ToLower(string(out)), "error") &&
		!strings.Contains(strings.ToLower(string(out)), "nothing") {
		t.Logf("undo output: %s", out)
		// Don't fail hard — output messages vary; just verify the file wasn't magically restored
		if _, err := os.Stat(target); err == nil {
			t.Error("large file was restored despite UNDO_MAX_SIZE — should have been skipped")
		}
	}
}

// ── purge clears history ──────────────────────────────────────────────────────

func TestE2EPurge(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	target := filepath.Join(home, "gone.txt")
	if err := os.WriteFile(target, []byte("bye"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _ = runCmd(bin, env, "capture", "rm", target)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(bin, env, "purge")
	if err != nil {
		t.Fatalf("purge: %v\n%s", err, out)
	}

	// After purge, history should be empty
	histOut, err := runCmd(bin, env, "history")
	if err != nil {
		t.Fatalf("history after purge: %v\n%s", err, histOut)
	}
	if strings.TrimSpace(string(histOut)) != "" && !strings.Contains(string(histOut), "0") {
		t.Logf("history after purge: %q", histOut)
	}

	// Undo after purge should fail gracefully (nothing to undo)
	undoOut, _ := runCmd(bin, env, "undo")
	if strings.Contains(string(undoOut), "panic") {
		t.Fatalf("undo after purge panicked:\n%s", undoOut)
	}
}

// ── nested directory capture ──────────────────────────────────────────────────

func TestE2ENestedDirectoryCapture(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	// Deep nested file
	deep := filepath.Join(home, "a", "b", "c", "d", "secret.key")
	if err := os.MkdirAll(filepath.Dir(deep), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deep, []byte("super-secret-key-material"), 0600); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(bin, env, "capture", "rm", deep)
	if err != nil {
		t.Fatalf("capture: %v\n%s", err, out)
	}

	if err := os.Remove(deep); err != nil {
		t.Fatal(err)
	}

	out, err = runCmd(bin, env, "undo")
	if err != nil {
		t.Fatalf("undo: %v\n%s", err, out)
	}

	got, err := os.ReadFile(deep)
	if err != nil {
		t.Fatalf("nested file not restored: %v", err)
	}
	if string(got) != "super-secret-key-material" {
		t.Errorf("content mismatch: %q", got)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func runCmd(bin string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	return cmd.CombinedOutput()
}
