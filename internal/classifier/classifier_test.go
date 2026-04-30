package classifier_test

import (
	"testing"

	"github.com/Ambar-13/undo/internal/classifier"
	"github.com/Ambar-13/undo/internal/store"
)

func TestClassifyRm(t *testing.T) {
	op, paths := classifier.Classify("rm -rf dist/ build/")
	if op != store.OpDelete {
		t.Errorf("op: got %q want %q", op, store.OpDelete)
	}
	if len(paths) != 2 || paths[0] != "dist/" || paths[1] != "build/" {
		t.Errorf("paths: %v", paths)
	}
}

func TestClassifyRmSimple(t *testing.T) {
	op, paths := classifier.Classify("rm foo.txt")
	if op != store.OpDelete {
		t.Errorf("op: got %q want delete", op)
	}
	if len(paths) != 1 || paths[0] != "foo.txt" {
		t.Errorf("paths: %v", paths)
	}
}

func TestClassifyMv(t *testing.T) {
	op, paths := classifier.Classify("mv old.txt new.txt")
	if op != store.OpMove {
		t.Errorf("op: got %q want move", op)
	}
	if len(paths) != 1 || paths[0] != "old.txt" {
		t.Errorf("paths (source only): %v", paths)
	}
}

func TestClassifyRedirect(t *testing.T) {
	op, paths := classifier.Classify("echo hello > output.txt")
	if op != store.OpOverwrite {
		t.Errorf("op: got %q want overwrite", op)
	}
	if len(paths) != 1 || paths[0] != "output.txt" {
		t.Errorf("paths: %v", paths)
	}
}

func TestClassifyTruncate(t *testing.T) {
	op, paths := classifier.Classify("truncate -s 0 myfile.log")
	if op != store.OpOverwrite {
		t.Errorf("op: got %q want overwrite", op)
	}
	if len(paths) != 1 || paths[0] != "myfile.log" {
		t.Errorf("paths: %v", paths)
	}
}

func TestClassifyUnknown(t *testing.T) {
	op, paths := classifier.Classify("ls -la")
	if op != store.OpUnknown {
		t.Errorf("expected OpUnknown, got %q", op)
	}
	if len(paths) != 0 {
		t.Errorf("expected no paths for unknown op")
	}
}

func TestClassifyGitClean(t *testing.T) {
	op, _ := classifier.Classify("git clean -fd")
	if op != store.OpDelete {
		t.Errorf("git clean should classify as delete, got %q", op)
	}
}
