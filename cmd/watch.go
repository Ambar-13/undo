package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch <script> [args...]",
	Short: "Run a script with undo capture hooks active",
	Long: `Wraps a shell script or Python script so every destructive command
inside it is captured and can be undone with 'undo undo'.

Supports .sh, .bash, and .py scripts.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

func runWatch(_ *cobra.Command, args []string) error {
	script := args[0]
	scriptArgs := args[1:]

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	ext := strings.ToLower(filepath.Ext(script))

	switch ext {
	case ".py":
		shimDir, err := writePythonShim(exe)
		if err != nil {
			return fmt.Errorf("python shim setup: %w", err)
		}
		defer os.RemoveAll(shimDir)

		pythonPath := shimDir
		if existing := os.Getenv("PYTHONPATH"); existing != "" {
			pythonPath = shimDir + ":" + existing
		}

		cmd = exec.Command("python3", append([]string{script}, scriptArgs...)...)
		cmd.Env = append(os.Environ(), "PYTHONPATH="+pythonPath)

	case ".js", ".mjs":
		shimPath, err := writeNodeShim(exe)
		if err != nil {
			return fmt.Errorf("node shim: %w", err)
		}
		defer os.Remove(shimPath)
		cmd = exec.Command("node", append([]string{"--require", shimPath, script}, scriptArgs...)...)
		cmd.Env = append(os.Environ(), "UNDO_BIN="+exe)

	case ".sh", ".bash":
		shellScript := filepath.Join(filepath.Dir(exe), "shell", "undo.bash")
		// If shell script not found next to binary, use embedded hook inline
		var hookLine string
		if _, err := os.Stat(shellScript); err == nil {
			hookLine = fmt.Sprintf(". %q && UNDO_QUIET=1", shellScript)
		} else {
			hookLine = fmt.Sprintf(
				`preexec_functions=(); _undo_preexec() { %q capture "$1" 2>&1; }; preexec_functions+=(_undo_preexec)`,
				exe,
			)
		}
		fullCmd := fmt.Sprintf("%s bash %q", hookLine, script)
		if len(scriptArgs) > 0 {
			quoted := make([]string, len(scriptArgs))
			for i, a := range scriptArgs {
				quoted[i] = fmt.Sprintf("%q", a)
			}
			fullCmd = fmt.Sprintf("%s bash %q %s", hookLine, script, strings.Join(quoted, " "))
		}
		cmd = exec.Command("bash", "-c", fullCmd)

	default:
		// Support natural forms like "undo watch python3 script.py" or
		// "undo watch node script.js" — reroute to the appropriate shim branch.
		base := filepath.Base(script)
		isPython := base == "python" || base == "python3" || base == "python2" ||
			strings.HasPrefix(base, "python3.") || strings.HasPrefix(base, "python2.")
		isNode := base == "node" || base == "nodejs"
		if (isPython || isNode) && len(scriptArgs) > 0 {
			// Re-invoke with the actual script file as first arg so the
			// extension-based switch above picks up the right shim.
			return runWatch(nil, append([]string{scriptArgs[0]}, scriptArgs[1:]...))
		}
		// Try executing directly (for scripts with shebangs)
		cmd = exec.Command(script, scriptArgs...)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writePythonShim(undoBin string) (string, error) {
	dir, err := os.MkdirTemp("", "undo-pyshim-*")
	if err != nil {
		return "", err
	}
	shim := fmt.Sprintf(`"""
undo sitecustomize.py — patches Python file APIs to capture files before modification.
Injected via PYTHONPATH by 'undo watch script.py'.
"""
import builtins as _builtins
import os as _os
import shutil as _shutil
import pathlib as _pathlib
import subprocess as _sp
import sys as _sys

_undo_bin = %q

def _capture(path):
    """Capture a file's current content before it is overwritten or deleted."""
    try:
        p = str(path)
        if _os.path.exists(p):
            _sp.run([_undo_bin, "capture-file", p],
                    stderr=_sys.stderr, env=_os.environ.copy())
    except Exception:
        pass  # never break the wrapped script

# ── builtins.open ──────────────────────────────────────────────────────────────
_orig_open = _builtins.open

def _patched_open(file, mode='r', *args, **kwargs):
    try:
        _mode = str(mode)
        if ('w' in _mode or 'x' in _mode) and 'r' not in _mode:
            _capture(file)
    except Exception:
        pass
    return _orig_open(file, mode, *args, **kwargs)

_builtins.open = _patched_open

# ── os.* ───────────────────────────────────────────────────────────────────────
_orig_unlink  = _os.unlink
_orig_remove  = _os.remove
_orig_rename  = _os.rename
_orig_replace = _os.replace

def _patched_unlink(path, **kw):
    _capture(path)
    return _orig_unlink(path, **kw)

def _patched_rename(src, dst, **kw):
    _capture(src)
    if _os.path.exists(str(dst)):
        _capture(dst)
    return _orig_rename(src, dst, **kw)

def _patched_replace(src, dst, **kw):
    _capture(src)
    _capture(dst)
    return _orig_replace(src, dst, **kw)

_os.unlink  = _patched_unlink
_os.remove  = _patched_unlink
_os.rename  = _patched_rename
_os.replace = _patched_replace

# ── shutil.* ───────────────────────────────────────────────────────────────────
_orig_copy     = _shutil.copy
_orig_copy2    = _shutil.copy2
_orig_copyfile = _shutil.copyfile
_orig_move     = _shutil.move
_orig_rmtree   = _shutil.rmtree

def _patched_copy(src, dst, **kw):
    if _os.path.isfile(str(dst)):
        _capture(dst)
    return _orig_copy(src, dst, **kw)

def _patched_copy2(src, dst, **kw):
    if _os.path.isfile(str(dst)):
        _capture(dst)
    return _orig_copy2(src, dst, **kw)

def _patched_copyfile(src, dst, **kw):
    if _os.path.exists(str(dst)):
        _capture(dst)
    return _orig_copyfile(src, dst, **kw)

def _patched_move(src, dst, **kw):
    _capture(src)
    if _os.path.exists(str(dst)):
        _capture(dst)
    return _orig_move(src, dst, **kw)

def _patched_rmtree(path, **kw):
    _capture(path)
    return _orig_rmtree(path, **kw)

_shutil.copy     = _patched_copy
_shutil.copy2    = _patched_copy2
_shutil.copyfile = _patched_copyfile
_shutil.move     = _patched_move
_shutil.rmtree   = _patched_rmtree

# ── pathlib.Path ───────────────────────────────────────────────────────────────
_orig_write_text  = _pathlib.Path.write_text
_orig_write_bytes = _pathlib.Path.write_bytes
_orig_path_unlink = _pathlib.Path.unlink
_orig_path_rename = _pathlib.Path.rename
_orig_path_replace = _pathlib.Path.replace

def _patched_write_text(self, data, *args, **kwargs):
    if self.exists():
        _capture(self)
    return _orig_write_text(self, data, *args, **kwargs)

def _patched_write_bytes(self, data):
    if self.exists():
        _capture(self)
    return _orig_write_bytes(self, data)

def _patched_path_unlink(self, missing_ok=False):
    _capture(self)
    return _orig_path_unlink(self, missing_ok=missing_ok)

def _patched_path_rename(self, target):
    _capture(self)
    if _pathlib.Path(str(target)).exists():
        _capture(target)
    return _orig_path_rename(self, target)

def _patched_path_replace(self, target):
    _capture(self)
    _capture(target)
    return _orig_path_replace(self, target)

_pathlib.Path.write_text  = _patched_write_text
_pathlib.Path.write_bytes = _patched_write_bytes
_pathlib.Path.unlink      = _patched_path_unlink
_pathlib.Path.rename      = _patched_path_rename
_pathlib.Path.replace     = _patched_path_replace
`, undoBin)
	return dir, os.WriteFile(filepath.Join(dir, "sitecustomize.py"), []byte(shim), 0644)
}

func writeNodeShim(undoBin string) (string, error) {
	f, err := os.CreateTemp("", "undo-node-shim-*.js")
	if err != nil {
		return "", err
	}
	defer f.Close()

	shim := fmt.Sprintf(`// undo node shim — patches fs APIs to capture files before modification
const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');

const UNDO_BIN = process.env.UNDO_BIN || %q;

function capture(filePath) {
  try {
    const p = String(filePath);
    if (fs.existsSync(p)) {
      execFileSync(UNDO_BIN, ['capture-file', p], {
        stdio: ['ignore', 'ignore', process.stderr],
        env: process.env
      });
    }
  } catch (_) {}
}

// fs.writeFile / writeFileSync
const _writeFile = fs.writeFile;
fs.writeFile = function(file, data, opts, cb) {
  if (typeof opts === 'function') { cb = opts; opts = {}; }
  capture(file);
  return _writeFile.call(this, file, data, opts, cb);
};
const _writeFileSync = fs.writeFileSync;
fs.writeFileSync = function(file, data, opts) {
  capture(file);
  return _writeFileSync.call(this, file, data, opts);
};

// fs.unlink / unlinkSync
const _unlink = fs.unlink;
fs.unlink = function(p, cb) { capture(p); return _unlink.call(this, p, cb); };
const _unlinkSync = fs.unlinkSync;
fs.unlinkSync = function(p) { capture(p); return _unlinkSync.call(this, p); };

// fs.rename / renameSync
const _rename = fs.rename;
fs.rename = function(o, n, cb) {
  capture(o);
  if (fs.existsSync(n)) capture(n);
  return _rename.call(this, o, n, cb);
};
const _renameSync = fs.renameSync;
fs.renameSync = function(o, n) {
  capture(o);
  if (fs.existsSync(n)) capture(n);
  return _renameSync.call(this, o, n);
};

// fs.copyFile / copyFileSync
const _copyFile = fs.copyFile;
fs.copyFile = function(src, dst, flags, cb) {
  if (typeof flags === 'function') { cb = flags; flags = 0; }
  if (fs.existsSync(dst)) capture(dst);
  return _copyFile.call(this, src, dst, flags, cb);
};
const _copyFileSync = fs.copyFileSync;
fs.copyFileSync = function(src, dst, flags) {
  if (fs.existsSync(dst)) capture(dst);
  return _copyFileSync.call(this, src, dst, flags);
};

// fs.truncate / truncateSync
const _truncate = fs.truncate;
fs.truncate = function(p, len, cb) {
  if (typeof len === 'function') { cb = len; len = 0; }
  capture(p);
  return _truncate.call(this, p, len, cb);
};
const _truncateSync = fs.truncateSync;
fs.truncateSync = function(p, len) {
  capture(p);
  return _truncateSync.call(this, p, len);
};
`, undoBin)
	_, err = f.WriteString(shim)
	return f.Name(), err
}
