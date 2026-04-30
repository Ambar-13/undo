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
git clone https://github.com/Ambar-13/undo
cd undo
go build -o undo .
sudo cp undo /usr/local/bin/
```

No separate shell file copy needed — shell scripts are embedded in the binary.

### Using `go install`

```bash
go install github.com/Ambar-13/undo@latest
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

`undo install` gives you two layers of coverage that compose:

**Layer 1 — shell hook** (typed commands in your interactive shell):

| Command | Captured |
|---|---|
| `rm file` | ✅ file content |
| `rm -rf dir/` | ✅ up to 1000 files |
| `mv src dst` | ✅ source + destination |
| `echo x > file` (redirect) | ✅ previous content |
| `truncate -s 0 log` | ✅ previous content |
| `shred file` | ✅ file content |
| `git clean -fd` | ⚠ paths resolved at runtime — undo records the command, reports if files weren't pre-captured |
| Files > 50 MB | ⚠ skipped, reported at capture time |

**Layer 2 — deep intercept** (compiled C library, active after `undo install`):

Intercepts `unlink`, `rename`, `truncate`, and `open(O_TRUNC)` at the libc level — before any destructive call completes, regardless of what spawned the process.

| Scenario | Captured |
|---|---|
| Subshells: `(rm foo)`, `$(rm foo)` | ✅ |
| Background jobs: `rm foo &` | ✅ |
| `make clean` spawning `rm` | ✅ |
| Python `subprocess.run(["rm", "file"])` | ✅ |
| C/Rust/Go programs calling `unlink()` directly | ✅ |
| Python `open("f", "w")` overwriting existing file | ✅ |
| `/bin/rm` on macOS (SIP-protected binary) | ⚠ covered by shell hook for typed commands; not interceptable as subprocess on macOS |

`⚠` means not captured — `undo` **tells you at capture time**, not at undo time.

`undo status` shows which layers are active.

### Claude Code / Cursor / any AI agent

```bash
undo install --claude-code
```

This does two things in `~/.claude/settings.json`:

1. **PreToolUse hook** — fires before every Write/Edit/Bash tool call, capturing files before Claude Code touches them.
2. **Deep intercept env** — injects the intercept library into every bash subprocess Claude Code spawns, regardless of how Claude Code was launched (Desktop app, VS Code, Spotlight — no shell inheritance needed).

After this, Claude Code's entire process tree is covered: direct file operations, bash commands, subshells inside bash commands, make, Python subprocess calls, everything.

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
