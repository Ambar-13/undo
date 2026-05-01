package classifier_test

import (
	"testing"

	"github.com/Ambar-13/undo/internal/classifier"
	"github.com/Ambar-13/undo/internal/store"
)

// ── rm ───────────────────────────────────────────────────────────────────────

func TestClassifyRm(t *testing.T) {
	op, paths := classifier.Classify("rm -rf dist/ build/")
	assertOp(t, op, store.OpDelete)
	assertPaths(t, paths, "dist/", "build/")
}

func TestClassifyRmSimple(t *testing.T) {
	op, paths := classifier.Classify("rm foo.txt")
	assertOp(t, op, store.OpDelete)
	assertPaths(t, paths, "foo.txt")
}

func TestClassifyRmNoFiles(t *testing.T) {
	op, paths := classifier.Classify("rm -rf")
	assertOp(t, op, store.OpDelete)
	if len(paths) != 0 {
		t.Errorf("expected no paths, got %v", paths)
	}
}

func TestClassifyRmLeadingWhitespace(t *testing.T) {
	op, paths := classifier.Classify("  rm foo.txt")
	assertOp(t, op, store.OpDelete)
	assertPaths(t, paths, "foo.txt")
}

func TestClassifyShred(t *testing.T) {
	op, paths := classifier.Classify("shred secret.key")
	assertOp(t, op, store.OpDelete)
	assertPaths(t, paths, "secret.key")
}

// ── mv ───────────────────────────────────────────────────────────────────────

func TestClassifyMv(t *testing.T) {
	op, paths := classifier.Classify("mv old.txt new.txt")
	assertOp(t, op, store.OpMove)
	assertPaths(t, paths, "old.txt") // source only, not destination
}

func TestClassifyMvMultipleSources(t *testing.T) {
	op, paths := classifier.Classify("mv a.txt b.txt dest/")
	assertOp(t, op, store.OpMove)
	assertPaths(t, paths, "a.txt", "b.txt") // dest/ excluded
}

func TestClassifyMvSingleArg(t *testing.T) {
	op, paths := classifier.Classify("mv lonely.txt")
	assertOp(t, op, store.OpMove)
	assertPaths(t, paths, "lonely.txt")
}

// ── cp ───────────────────────────────────────────────────────────────────────

func TestClassifyCp(t *testing.T) {
	op, paths := classifier.Classify("cp src.txt dst.txt")
	assertOp(t, op, store.OpOverwrite)
	assertPaths(t, paths, "dst.txt") // destination only
}

func TestClassifyCpMultiple(t *testing.T) {
	op, paths := classifier.Classify("cp a.txt b.txt dest/")
	assertOp(t, op, store.OpOverwrite)
	assertPaths(t, paths, "dest/") // last arg is destination
}

// ── redirect ─────────────────────────────────────────────────────────────────

func TestClassifyRedirect(t *testing.T) {
	op, paths := classifier.Classify("echo hello > output.txt")
	assertOp(t, op, store.OpOverwrite)
	assertPaths(t, paths, "output.txt")
}

func TestClassifyRedirectNoSpace(t *testing.T) {
	op, paths := classifier.Classify("echo hello >output.txt")
	assertOp(t, op, store.OpOverwrite)
	assertPaths(t, paths, "output.txt")
}

func TestClassifyRedirectCat(t *testing.T) {
	op, paths := classifier.Classify("cat template > result.txt")
	assertOp(t, op, store.OpOverwrite)
	assertPaths(t, paths, "result.txt")
}

// ── truncate ─────────────────────────────────────────────────────────────────

func TestClassifyTruncate(t *testing.T) {
	op, paths := classifier.Classify("truncate -s 0 myfile.log")
	assertOp(t, op, store.OpOverwrite)
	assertPaths(t, paths, "myfile.log")
}

func TestClassifyTruncateSkipsFlagValue(t *testing.T) {
	// -s takes a value argument; that value must not be treated as a file path
	op, paths := classifier.Classify("truncate -s 100 app.log")
	assertOp(t, op, store.OpOverwrite)
	assertPaths(t, paths, "app.log")
}

func TestClassifyTruncateMultipleFiles(t *testing.T) {
	op, paths := classifier.Classify("truncate -s 0 a.log b.log")
	assertOp(t, op, store.OpOverwrite)
	assertPaths(t, paths, "a.log", "b.log")
}

// ── git clean ────────────────────────────────────────────────────────────────

func TestClassifyGitClean(t *testing.T) {
	op, _ := classifier.Classify("git clean -fd")
	assertOp(t, op, store.OpDelete)
}

func TestClassifyGitCleanVariants(t *testing.T) {
	for _, cmd := range []string{
		"git clean -f",
		"git clean -fdx",
		"git clean -fd .",
		"git clean -fX",
	} {
		op, _ := classifier.Classify(cmd)
		if op != store.OpDelete {
			t.Errorf("cmd %q: expected OpDelete, got %q", cmd, op)
		}
	}
}

func TestIsGitClean(t *testing.T) {
	cases := map[string]bool{
		"git clean -fd":   true,
		"git clean -f":    true,
		"  git clean -fd": true,
		"rm -rf":          false,
		"git status":      false,
		"git checkout .":  false,
	}
	for cmd, want := range cases {
		if got := classifier.IsGitClean(cmd); got != want {
			t.Errorf("IsGitClean(%q) = %v, want %v", cmd, got, want)
		}
	}
}

// ── unknown ──────────────────────────────────────────────────────────────────

func TestClassifyUnknown(t *testing.T) {
	for _, cmd := range []string{"ls -la", "cat file.txt", "grep foo bar", "git status", "make build"} {
		op, paths := classifier.Classify(cmd)
		if op != store.OpUnknown {
			t.Errorf("cmd %q: expected OpUnknown, got %q", cmd, op)
		}
		if len(paths) != 0 {
			t.Errorf("cmd %q: expected no paths, got %v", cmd, paths)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	op, paths := classifier.Classify("")
	if op != store.OpUnknown {
		t.Errorf("empty: expected OpUnknown, got %q", op)
	}
	if len(paths) != 0 {
		t.Errorf("empty: expected no paths, got %v", paths)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func assertOp(t *testing.T, got, want store.OpType) {
	t.Helper()
	if got != want {
		t.Errorf("op: got %q want %q", got, want)
	}
}

func assertPaths(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("paths len: got %d want %d: %v", len(got), len(want), got)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("paths[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}
