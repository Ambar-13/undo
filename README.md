# undo

**Filesystem undo for your terminal.** Capture `rm`, `mv`, and overwrites before they happen — and restore them with a single `undo`.

Works with your shell, your scripts, and your AI agents (Claude Code, Cursor, Aider).

---

## The problem

You run `rm -rf dist/` or an AI agent issues `mv config.json config.json.bak` and overwrites your work. Git doesn't track it. The recycle bin doesn't cover it. There's no undo.

`undo` fills that gap.

---

## How it works

A shell hook calls `undo capture` before every command. If the command is destructive (`rm`, `mv`, `>`, `truncate`, `shred`, `git clean`), undo snapshots the affected files into a content-addressed object store (`~/.undo/`) and appends a journal entry. When you run `undo`, it restores from the snapshot.

```
$ rm important-config.json
  · captured  important-config.json  →  undo

$ undo
  ✓ restored  rm important-config.json
```

History and object store are per-session. This is a **safety net**, not a backup.

---

## Install

### Homebrew (macOS/Linux) — coming soon

```bash
brew install undo   # formula in progress
```

### From source (requires Go 1.22+)

```bash
git clone https://github.com/Ambar13/undo
cd undo
go build -o undo .
sudo cp undo /usr/local/bin/
```

No separate shell file copy needed — shell scripts are embedded in the binary.

### Using `go install`

```bash
go install github.com/Ambar13/undo@latest
```

### Hook into your shell

```bash
undo install
```

This extracts the shell integration to `~/.config/undo/shell/` and adds a `source` line to your rc file. Then restart your shell, or `source ~/.zshrc`.

**Bash** (requires [bash-preexec](https://github.com/rcaloras/bash-preexec)):

```bash
# Install bash-preexec first:
curl https://raw.githubusercontent.com/rcaloras/bash-preexec/master/bash-preexec.sh -o ~/.bash-preexec.sh
echo '[[ -f ~/.bash-preexec.sh ]] && source ~/.bash-preexec.sh' >> ~/.bashrc
undo install
```

### Verify it's active

```bash
undo status
```

---

## Usage

```bash
# Undo the last operation
undo

# Undo the last 3 operations (shows preview, asks confirmation)
undo 3

# See what's been captured this session
undo history

# Check if undo is active in the current session
undo status

# Wrap a shell script so rm/mv/overwrites inside it are captured
undo watch deploy.sh

# Wrap a Python script (captures os.unlink, os.remove, os.rename)
undo watch cleanup.py

# Wrap a JavaScript/Node.js script
undo watch script.js
```

---

## What gets captured

| Command | Op | Captured |
|---|---|---|
| `rm file` | delete | ✅ file content |
| `rm -rf dir/` | delete | ✅ up to 1000 files |
| `mv src dst` | move | ✅ source file |
| `echo x > file` | overwrite | ✅ previous content |
| `truncate -s 0 log` | overwrite | ✅ previous content |
| `shred file` | delete | ✅ file content |
| `git clean -fd` | delete | ⚠ paths not known — undo will report an error |
| Files > 50 MB | any | ⚠ skipped (shown at capture time) |
| Commands in subshells | any | ⚠ not captured (see below) |
| Python `subprocess.run(["rm", ...])` | delete | ⚠ not captured (see below) |

`⚠` means not captured — `undo` **tells you at capture time**, not at undo time.

### Shell scope

The preexec hook only sees commands typed directly in your interactive shell. It does **not** capture:

- Commands in subshells: `(rm foo)`, `$(rm foo)`, background jobs
- Scripts that spawn sub-processes externally (e.g. a Makefile running `rm` via `make`)

`undo watch script.sh` captures commands run at the top-level bash session wrapping the script, but not commands in sub-processes the script spawns.

### Python scope

`undo watch script.py` patches `os.unlink`, `os.remove`, and `os.rename`. It does **not** capture:

- `subprocess.run(["rm", ...])` or any other subprocess calls
- C extensions that call `unlink` directly

If your Python script uses subprocess to delete files, wrap those calls with `os.remove()` instead.

### Claude Code / Cursor integration

For the deepest coverage — capturing every file Write, Edit, and Bash command Claude Code runs — install the tool-level hook:

```bash
undo install --claude-code
```

This writes a `PreToolUse` hook into `~/.claude/settings.json`. From that point, **every file Claude Code writes, edits, or touches via bash is captured before the change happens** — including direct file writes that bypass the shell entirely.

Works with Claude Code. Cursor and other agents with hook APIs: contributions welcome.

---

## AI agent support

`undo` detects which AI agent ran a command and attributes it in `undo history`:

| Env var | Source |
|---|---|
| `CLAUDE_CODE_SESSION_ID` | `claude` |
| `CURSOR_SESSION_ID` | `cursor` |
| `AIDER_SESSION_ID` | `aider` |
| `COPILOT_SESSION_ID` | `copilot` |
| *(none)* | `you` |

```
TIME     SOURCE  OP      COMMAND
----     ------  --      -------
10:04PM  claude  delete  rm -rf node_modules/
10:06PM  you     move    mv config.json config.bak
```

---

## Design principles

- **Warn at capture time, not undo time.** If a file can't be captured (too large, directory too deep), you see `⚠ not captured` immediately — not when it's too late.
- **Safety net, not backup.** History from previous sessions is not accessible from `undo` (it reads the current session's journal). Object data persists in `~/.undo/` until you delete it.
- **Zero config.** One install command. No daemons, no background processes.
- **Explicit cleanup.** Run `undo purge` to remove sessions older than 24 hours and any unreferenced objects. Use `undo purge --all` to wipe everything.
- **Fast.** The capture hook adds < 50 ms to each command for typical files.

---

## Storage layout

```
~/.undo/
├── objects/          # SHA-256 content-addressed store (git-style)
│   └── ab/           # first 2 chars of hash
│       └── cdef...   # remaining 62 chars
└── sessions/
    └── <ppid>.journal  # append-only JSONL, one entry per captured command
```

---

## License

MIT — see [LICENSE](LICENSE).
