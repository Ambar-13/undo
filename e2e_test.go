package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "undo")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "/Users/ambar/untitled folder 2/undo"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	return bin
}

func TestE2ECaptureAndUndo(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()

	// Create test file
	target := filepath.Join(home, "important.txt")
	if err := os.WriteFile(target, []byte("do not lose this"), 0644); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "HOME="+home)

	// Capture "rm important.txt"
	captureOut, err := runCmd(bin, env, "capture", "rm", target)
	if err != nil {
		t.Fatalf("capture failed: %v\n%s", err, captureOut)
	}

	// Delete the file (simulate what rm would do)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	// Verify it's gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("file should not exist after removal")
	}

	// Undo
	undoOut, err := runCmd(bin, env, "undo")
	if err != nil {
		t.Fatalf("undo failed: %v\n%s", err, undoOut)
	}

	// Verify restored
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("file not restored: %v\nundo output: %s", err, undoOut)
	}
	if string(content) != "do not lose this" {
		t.Errorf("wrong content after restore: %q", content)
	}
}

func TestE2EHistoryShowsEntry(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home)

	target := filepath.Join(home, "tracked.txt")
	os.WriteFile(target, []byte("data"), 0644)

	runCmd(bin, env, "capture", "rm", target)

	out, err := runCmd(bin, env, "history")
	if err != nil {
		t.Fatalf("history failed: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Error("history output is empty, expected at least one entry")
	}
}

func runCmd(bin string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	return cmd.CombinedOutput()
}
