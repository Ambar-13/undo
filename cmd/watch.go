package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Build environment with LD_PRELOAD/DYLD_INSERT_LIBRARIES injected so every
	// child process inherits the intercept library — subshells, make, subprocess
	// calls, C programs inside the script are all covered, not just top-level cmds.
	base := deepInterceptEnv(exe)

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
		cmd.Env = append(base, "PYTHONPATH="+pythonPath)

	case ".js", ".mjs":
		shimPath, err := writeNodeShim(exe)
		if err != nil {
			return fmt.Errorf("node shim: %w", err)
		}
		defer os.Remove(shimPath)
		cmd = exec.Command("node", append([]string{"--require", shimPath, script}, scriptArgs...)...)
		cmd.Env = base

	case ".sh", ".bash":
		shellScript := filepath.Join(filepath.Dir(exe), "shell", "undo.bash")
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
		cmd.Env = base

	default:
		scriptBase := filepath.Base(script)
		isPython := scriptBase == "python" || scriptBase == "python3" || scriptBase == "python2" ||
			strings.HasPrefix(scriptBase, "python3.") || strings.HasPrefix(scriptBase, "python2.")
		isNode := scriptBase == "node" || scriptBase == "nodejs"
		if (isPython || isNode) && len(scriptArgs) > 0 {
			return runWatch(nil, append([]string{scriptArgs[0]}, scriptArgs[1:]...))
		}
		cmd = exec.Command(script, scriptArgs...)
		cmd.Env = base
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// deepInterceptEnv returns os.Environ() with LD_PRELOAD (Linux) or
// DYLD_INSERT_LIBRARIES (macOS) set to the compiled intercept library,
// plus UNDO_BIN pointing at the current executable. Every child process
// that inherits this environment gets libc-level capture coverage.
// Returns plain os.Environ() if the library hasn't been compiled yet.
func deepInterceptEnv(exe string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Environ()
	}

	libName := "libundo_intercept.so"
	envKey := "LD_PRELOAD"
	if runtime.GOOS == "darwin" {
		libName = "libundo_intercept.dylib"
		envKey = "DYLD_INSERT_LIBRARIES"
	}
	libPath := filepath.Join(home, ".config", "undo", "lib", libName)
	if _, err := os.Stat(libPath); err != nil {
		return os.Environ()
	}

	src := os.Environ()
	out := make([]string, 0, len(src)+2)
	for _, e := range src {
		if strings.HasPrefix(e, envKey+"=") || strings.HasPrefix(e, "UNDO_BIN=") {
			continue
		}
		out = append(out, e)
	}
	out = append(out, envKey+"="+libPath)
	out = append(out, "UNDO_BIN="+exe)
	return out
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

# ── subprocess.* ───────────────────────────────────────────────────────────────
# Captures files that would be destroyed by shell commands run from Python.
# Only the list-form is analyzed (string-form is too risky to parse).

_DESTRUCTIVE_CMDS = {
    'rm': 'delete',  'remove': 'delete',  'del': 'delete',
    'mv': 'move',    'move': 'move',
    'cp': 'copy',    'copy': 'copy',
    'truncate': 'truncate',
    'shred': 'delete', 'wipe': 'delete',
}

def _analyze_subprocess_cmd(cmd):
    try:
        if not isinstance(cmd, (list, tuple)) or not cmd:
            return
        base = _os.path.basename(str(cmd[0]))
        kind = _DESTRUCTIVE_CMDS.get(base)
        if kind is None:
            return
        # Filter out flag arguments (start with -)
        file_args = [str(a) for a in cmd[1:] if str(a) and not str(a).startswith('-')]
        if kind == 'delete':
            for p in file_args:
                _capture(p)
        elif kind == 'move':
            for p in file_args:
                _capture(p)
        elif kind == 'copy':
            # Capture destination only (last non-flag arg)
            if file_args:
                _capture(file_args[-1])
        elif kind == 'truncate':
            for p in file_args:
                _capture(p)
    except Exception:
        pass

_orig_sp_run          = _sp.run
_orig_sp_call         = _sp.call
_orig_sp_check_call   = _sp.check_call
_orig_sp_check_output = _sp.check_output
_orig_Popen_init      = _sp.Popen.__init__

def _patched_sp_run(cmd, *a, **kw):
    _analyze_subprocess_cmd(cmd)
    return _orig_sp_run(cmd, *a, **kw)

def _patched_sp_call(cmd, *a, **kw):
    _analyze_subprocess_cmd(cmd)
    return _orig_sp_call(cmd, *a, **kw)

def _patched_sp_check_call(cmd, *a, **kw):
    _analyze_subprocess_cmd(cmd)
    return _orig_sp_check_call(cmd, *a, **kw)

def _patched_sp_check_output(cmd, *a, **kw):
    _analyze_subprocess_cmd(cmd)
    return _orig_sp_check_output(cmd, *a, **kw)

def _patched_Popen_init(self, cmd, *a, **kw):
    _analyze_subprocess_cmd(cmd)
    return _orig_Popen_init(self, cmd, *a, **kw)

_sp.run           = _patched_sp_run
_sp.call          = _patched_sp_call
_sp.check_call    = _patched_sp_check_call
_sp.check_output  = _patched_sp_check_output
_sp.Popen.__init__ = _patched_Popen_init
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
