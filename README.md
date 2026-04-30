# undo

Your terminal has no undo button. This is that button.

`rm important.json`. Gone. Git doesn't know about it. The recycle bin definitely doesn't. There's no recovery path. You just lost the file.

`undo` fixes this. It hooks into your shell and silently snapshots files before they're destroyed. When something goes wrong, one command brings them back.

```
$ rm important-config.json
  · captured  important-config.json  →  undo

$ undo
  ✓ restored  rm important-config.json
```

Works with your shell, your scripts, and your AI agents (Claude Code, Cursor, Aider).

> **Already deleted something?** `undo` only works if it was running *before* the deletion. If it wasn't running yet, try `photorec`, `testdisk`, or check Trash and Time Machine first.

---

## When you actually need this

**The classic typo.**
`rm -rf ./dist` from one directory up. One second. No recovery. Happens to experienced developers.

**The AI cleanup script with a bug.**
You ask Claude to write a cleanup script. It removes files matching the wrong pattern. Calls `shutil.rmtree` on the wrong directory. Opens existing files with `"w"` mode and wipes them. The AI wrote the code. It doesn't know what was inside those files. It can't undo what its own code did. `undo` had the bytes before the script ran.

**The file the AI can never reconstruct.**
Model weights. SQLite databases. Compiled artifacts. Images. PDFs. If an AI agent runs `rm model.pt` or a script wipes your local database, asking it to undo gets you: *"I can't recreate that file."* `undo` had it.

**The config that isn't in git.**
`.env`. `~/.ssh/config`. `database.yml`. Credentials. Dotfiles. Gitignored or outside any repo entirely. One wrong move from a cleanup script and they're gone with nowhere to recover from.

**The command you typed, not the AI.**
You ran `rm config.json`. The AI has no knowledge of it. Asking it to undo your terminal commands is asking it to hallucinate what was there.

**The next morning.**
You come back. The AI's context is gone. It has no memory of what it deleted or overwrote yesterday. `undo` has the journal and the bytes.

**The deploy script that failed halfway.**
`rm -rf dist/ && rm -rf .cache/` before rebuild. Build fails. Now you have nothing. The AI ran one bash command. It has no idea what was inside those directories.

**Even when the AI could undo it.**
An AI agent still in context can try to restore a file it just modified. But it rewrites from memory. Possibly slightly different, possibly "improved," never byte-for-byte identical. `undo` makes restore mean what Ctrl+Z means in an editor: instant, exact, no inference. One command, original bytes back, no conversation required.

**When you don't need this:** if everything you care about is git-tracked source code, `git checkout` already covers you. `undo` fills the gap git doesn't: configs, data, binaries, dotfiles, and anything destroyed by code you ran rather than code you wrote.

---

## Install

**This tool requires Go.** Check if you have it:

```bash
go version   # prints "go1.22.x ..." if installed; "command not found" if not
```

If you don't have Go, install it from [go.dev/dl](https://go.dev/dl). Pick the macOS or Linux installer. It takes about 2 minutes. Then come back here.

### Step 1: Install the binary

```bash
go install github.com/Ambar-13/undo@latest
```

This downloads, compiles, and installs the `undo` binary to `~/go/bin/`. If `undo` isn't found after this, add Go's bin directory to your PATH:

```bash
# Add to ~/.zshrc or ~/.bashrc, then restart your shell:
export PATH="$PATH:$HOME/go/bin"
```

<details>
<summary>Build from source instead</summary>

```bash
git clone https://github.com/Ambar-13/undo
cd undo
go build -o undo .
sudo cp undo /usr/local/bin/
```

</details>

### Step 2: Hook into your shell

Once `undo` is in your PATH, run this once:

```bash
# Shell only (interactive terminal coverage)
undo install

# Shell + Claude Code (recommended if you use Claude Code)
undo install --claude-code
```

`undo install` wires `undo` into your shell so it activates automatically for every future command. It adds a `source` line to your `~/.zshrc` (or `~/.bashrc`). Then restart your shell, or:

```bash
source ~/.zshrc   # or ~/.bashrc
```

`--claude-code` does everything above, plus wires up Claude Code hooks (see [Claude Code / Cursor / any AI agent](#claude-code--cursor--any-ai-agent) below).

**Bash** (requires [bash-preexec](https://github.com/rcaloras/bash-preexec)):

```bash
# Install bash-preexec first:
curl https://raw.githubusercontent.com/rcaloras/bash-preexec/master/bash-preexec.sh -o ~/.bash-preexec.sh
echo '[[ -f ~/.bash-preexec.sh ]] && source ~/.bash-preexec.sh' >> ~/.bashrc
undo install
```

### Step 3: Verify it's active

```bash
undo status
```

> **Homebrew formula** is in progress. Not available yet.

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

# Wrap a Python script (captures file ops, shutil.*, subprocess.run, and more)
undo watch cleanup.py

# Wrap a JavaScript/Node.js script
undo watch script.js

# Remove stored data older than 24 hours (run periodically to free disk)
undo purge

# Remove all stored data
undo purge --all
```

---

## What gets captured

`undo install` gives you two layers of coverage that compose:

**Layer 1: shell hook** (typed commands in your interactive shell):

| Command | Captured |
|---|---|
| `rm file` | ✅ file content |
| `rm -rf dir/` | ✅ up to 1000 files |
| `mv src dst` | ✅ source + destination |
| `echo x > file` (redirect) | ✅ previous content |
| `truncate -s 0 log` | ✅ previous content |
| `shred file` | ✅ file content |
| `git clean -fd` | ✅ dry-run previews paths before deletion, captures each file |
| Files > 50 MB | ⚠ skipped. Raise the limit with `UNDO_MAX_SIZE=500MB`, or `=0` for no limit (see note below) |

> **UNDO_MAX_SIZE:** To set persistently, add `export UNDO_MAX_SIZE=500MB` to your `.zshrc` / `.bashrc`. Large limits mean large captures. A 2 GB model weight captured on every overwrite will fill disk quickly. Run `undo purge` periodically or set a reasonable limit for your use case.

**Layer 2: deep intercept** (compiled C library, active after `undo install`):

Intercepts `unlink`, `rename`, `truncate`, and `open(O_TRUNC)` using `LD_PRELOAD` (Linux) / `DYLD_INSERT_LIBRARIES` (macOS). Fires before any destructive call completes, regardless of what spawned the process.

| Scenario | Captured |
|---|---|
| Subshells: `(rm foo)`, `$(rm foo)` | ✅ |
| Background jobs: `rm foo &` | ✅ |
| `make clean` spawning `rm` | ✅ |
| Python `subprocess.run(["rm", "file"])` | ✅ |
| C/Rust/Go programs calling `unlink()` directly | ✅ |
| Python `open("f", "w")` overwriting existing file | ✅ |
| `/bin/rm` on macOS (SIP-protected binary) | ⚠ covered by shell hook for typed commands; not interceptable as subprocess on macOS |

`⚠` means not captured. `undo` **tells you at capture time**, not at undo time.

`undo status` shows which layers are active.

### Claude Code / Cursor / any AI agent

```bash
undo install --claude-code
```

This does two things in `~/.claude/settings.json`:

1. **PreToolUse hook:** fires before every Write/Edit/Bash tool call, capturing files before Claude Code touches them.
2. **Deep intercept env:** injects the intercept library into every bash subprocess Claude Code spawns, regardless of how Claude Code was launched (Desktop app, VS Code, Spotlight).

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

- **Warn at capture time, not undo time.** If a file can't be captured (too large, directory too deep), you see `⚠ not captured` immediately. Not when it's too late.
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

MIT. See [LICENSE](LICENSE).
