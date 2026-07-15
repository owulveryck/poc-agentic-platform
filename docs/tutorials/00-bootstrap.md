# Tutorial 0 — bootstrap the platform on your machine

> **Goal**: from a fresh laptop to "everything works, once" in about 10
> minutes. Every subsequent tutorial's prereqs collapse to a link back
> here.
>
> What this tutorial installs is **per-machine**, not per-project: the
> gateway, the adapter binaries, and the MCP server registration.
> Per-project wiring (hooks, contract file, skills) is covered in the
> tutorial you're running that day.
>
> Prerequisites: Go 1.25+, one of:
> - the official **GitHub Copilot desktop app** and/or `gh copilot` CLI,
> - **VS Code** + the GitHub Copilot extensions,
> - the **Claude Code** CLI.
> If you want to install skills (tutorials 6, 8), also
> [APM](https://github.com/microsoft/apm) ≥ 0.23.

## Step 1 — Clone the platform once

You will build binaries from this checkout; keep it on disk so future
tutorials can reference it.

```bash
mkdir -p ~/src && cd ~/src
git clone https://github.com/owulveryck/poc-agentic-platform
cd poc-agentic-platform
```

## Step 2 — Build the binaries onto your `PATH`

Four binaries, one `PATH` directory. `~/.local/bin` is the convention;
substitute what your shell already has on `PATH`.

```bash
mkdir -p ~/.local/bin

# The gateway itself
go build -o ~/.local/bin/ppg               ./cmd/ppg

# MCP server: bridges the gateway's HTTP API to agents that speak MCP
# (Claude Code, Copilot CLI/desktop, VS Code Copilot Chat)
go build -o ~/.local/bin/ppg-mcp-server    ./adapters/claudecode/mcpserver

# PreToolUse hook for the GitHub Copilot surfaces
go build -o ~/.local/bin/ppg-copilot-guard ./adapters/copilot/guard

# PreToolUse hook for Claude Code (skip if you don't use Claude Code)
go build -o ~/.local/bin/ppg-guard         ./adapters/claudecode/guard
```

Verify each is on `PATH`:

```bash
which ppg ppg-mcp-server ppg-copilot-guard ppg-guard
```

## Step 3 — Register the MCP server (per agent surface)

The MCP config location depends on which surface you use. Register once
per surface you plan to use; the entry is user-scoped so it applies to
every project on this machine.

### Copilot desktop app or `gh copilot` CLI

Both surfaces read `~/.copilot/mcp-config.json`. Two equivalent ways
to write it:

> ⚠️ **macOS GUI PATH gotcha** — the Copilot desktop app is a GUI
> process, and macOS GUI processes **do not inherit your shell's
> `PATH`**. `~/.local/bin` (and often `~/.local/bin` even after
> `.zshrc` edits) is *not* on the PATH the app sees when it spawns
> the MCP subprocess. Config values like `"command": "ppg-mcp-server"`
> silently fail with a "connecting…" loop. **Use a fully-expanded
> absolute path** (`$HOME/.local/bin/ppg-mcp-server`, expanded by
> the shell before it lands in the JSON) or install the binaries
> under `/usr/local/bin/` (macOS GUI PATH includes it by default).

**A. With the `copilot` CLI** (shortest, if it is installed):

```bash
copilot mcp add ppg --env PPG_URL=http://localhost:8765 \
  -- "$HOME/.local/bin/ppg-mcp-server"
copilot mcp list       # → ppg  connected
```

**B. Without the CLI** — hand-edit the file (the desktop app reads it
directly, no CLI required). Note the `<<EOF` (no quotes) so `$HOME`
expands to your literal absolute path before landing in the JSON:

```bash
mkdir -p ~/.copilot

if [ -f ~/.copilot/mcp-config.json ]; then
  echo "⚠️  ~/.copilot/mcp-config.json already exists — NOT overwriting." >&2
  echo "   Merge this block into its 'mcpServers' object by hand:" >&2
  cat >&2 <<EOF
    "ppg": {
      "type": "stdio",
      "command": "$HOME/.local/bin/ppg-mcp-server",
      "env": { "PPG_URL": "http://localhost:8765" },
      "tools": ["*"]
    }
EOF
else
  cat > ~/.copilot/mcp-config.json <<EOF
{
  "mcpServers": {
    "ppg": {
      "type": "stdio",
      "command": "$HOME/.local/bin/ppg-mcp-server",
      "env": { "PPG_URL": "http://localhost:8765" },
      "tools": ["*"]
    }
  }
}
EOF
fi

cat ~/.copilot/mcp-config.json    # verify — you should see /Users/<you>/.local/bin/... in "command"
```

### Claude Code

```bash
claude mcp add ppg --scope user --env PPG_URL=http://localhost:8765 \
  -- "$HOME/.local/bin/ppg-mcp-server"
claude mcp list        # → ppg  connected
```

(Absolute path recommended for consistency with the Copilot case above
and because Claude Code as a launchd agent would face the same GUI-
PATH restriction if launched from Finder rather than a terminal.)

### VS Code Copilot Chat

MCP config in VS Code is **workspace-scoped**, not user-scoped. It lives
in `.vscode/mcp.json` inside each project — so this is done per-project,
not here. Every downstream tutorial that targets VS Code creates that
file itself. See
[tutorial 7](07-copilot-end-to-end.md#vs-code-copilot-chat-workspace-mcp)
for the schema.

## Step 4 — Start the gateway (one terminal, leave it running)

```bash
ppg -addr :8765
```

You should see:

```
ADR store loaded: 5 invariants
Plan linter ready: 5 policies
Skill governance linter ready
Platform Planning Gateway listening on :8765
```

Every downstream tutorial expects this on `:8765`. Keep this terminal
open — or run the gateway as a launchd/systemd service (see the
appendix below).

## Step 5 — Sanity-check the whole wiring

One tool call from each agent surface confirms binaries + registration
+ gateway are working end-to-end.

### From Copilot

Open any folder in the Copilot app (or start `gh copilot`) and chat:

> Call the ppg MCP tool `get_platform_guidelines_for_intent` with intent
> "test bootstrap" and repository_context `{"name":"bootstrap-check",
> "tech_stack":["Go"]}`. Show me the JSON result.

Expected: a JSON blob containing `architectural_invariants` with at
least ADR-042 and ADR-070. If you get "tool not found", the MCP
registration didn't take — recheck step 3.

### From Claude Code

Same prompt in a `claude` session:

```
> Call the ppg MCP tool get_platform_guidelines_for_intent with intent
> "test bootstrap" and repository_context {"name":"bootstrap-check",
> "tech_stack":["Go"]}. Show me the JSON result.
```

Same expected output.

## Step 6 — (Optional) Install the demo skills

For tutorials 6 and 8, install the demo skill package once per project
you want the skills in:

```bash
# In a target project's root
apm install ~/src/poc-agentic-platform/demo --target copilot   # → .agents/skills/
apm install ~/src/poc-agentic-platform/demo --target claude    # → .claude/skills/
```

Ships three skills: `ppg-tutorial`, `add-payment-method`, and
`design-system`.

## Per-machine vs. per-project cheat sheet

| Per-machine (this tutorial) | Per-project (each tutorial you run) |
|---|---|
| Clone `poc-agentic-platform` | `git init` a target project |
| Build the four binaries onto `PATH` | Enable hooks in `.github/hooks/` (Copilot) or `.claude/settings.json` (Claude) |
| Register the MCP server (user scope) | Preflight `.github/copilot-instructions.md` for the current intent |
| Start `ppg -addr :8765` | (Optionally) `apm install` a skill |

## Troubleshooting

Common failure modes we've hit while shipping the tutorials:

- **`.vscode/mcp.json` is ignored by the Copilot desktop app / CLI.**
  Those surfaces read `~/.copilot/mcp-config.json`. VS Code reads
  `.vscode/mcp.json`. If your Copilot session can't find the `ppg`
  tools, check the right file for your surface.

- **Copilot shows the `ppg` server but loops on "connecting…"**
  Almost always a **PATH problem**: the Copilot desktop app is a GUI
  process, so it inherits the macOS *login* PATH, not your shell's
  PATH. `~/.local/bin` is not on the login PATH. Steps to check, in
  order:
    1. Verify the config uses a **fully-expanded absolute path** (open
       `~/.copilot/mcp-config.json` — the `command` should read
       `/Users/<you>/.local/bin/ppg-mcp-server`, not
       `~/.local/bin/…` and not `ppg-mcp-server` alone).
    2. Verify the binary starts standalone:
       ```bash
       python3 -c "
       import json,subprocess,select
       p=subprocess.Popen(['$HOME/.local/bin/ppg-mcp-server'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,env={'PATH':'/usr/bin:/bin','PPG_URL':'http://localhost:8765'})
       p.stdin.write(b'{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"clientInfo\":{\"name\":\"t\",\"version\":\"1\"}}}\n')
       p.stdin.flush()
       r,_,_=select.select([p.stdout],[],[],3)
       print('OK:' if r else 'HANG', p.stdout.readline().decode() if r else '')
       p.kill()"
       ```
       Expected: `OK: {"jsonrpc":"2.0","id":1,"result":{...}}`. If
       you see `HANG`, the binary itself is broken (rebuild). If you
       see `FileNotFoundError`, path is wrong.
    3. Clear macOS quarantine on the binary (compiled binaries can
       carry `com.apple.quarantine` if downloaded via a browser; not
       normally set by `go build`, but worth checking):
       `xattr -d com.apple.quarantine ~/.local/bin/ppg-mcp-server`.
    4. Restart the Copilot session (the MCP client only re-reads
       config at session start).
  If none of that helps, the Copilot logs are typically under
  `~/Library/Application Support/*Copilot*/logs/` or
  `~/Library/Logs/*Copilot*/`. Search there for the string
  `ppg-mcp-server` to find the actual error.

- **`ppg-copilot-guard: command not found` inside a hook.** Same
  GUI-PATH story as above, but for the hook subprocess. Use an
  absolute path (`$HOME/.local/bin/ppg-copilot-guard`) in the
  `.github/hooks/*.json` `command` field, or install the binaries
  under `/usr/local/bin/` which is on the macOS login PATH by
  default.

- **`PPG_URL` mismatch.** If you started the gateway on a different
  port than the MCP server was registered with, the MCP call succeeds
  but returns nothing. Re-register the MCP server with the correct
  `--env PPG_URL=…`, or restart the gateway on the expected port.

- **Gateway startup shows `4 invariants` instead of `5`.** You are on
  a checkout that predates ADR-090. `git pull`.

## Appendix — running the gateway as a service

macOS (launchd) example: create
`~/Library/LaunchAgents/dev.ppg.plist` with a program pointing at
`~/.local/bin/ppg` and args `["-addr", ":8765"]`. `launchctl load` it
once. The gateway is holding-cost-free (no external dependencies) — you
can leave it running.

**✅ Done.** You have the platform installed once for every project on
this machine. From here:

- [Tutorial 1](01-first-planning-cycle.md) — the amplified loop by
  hand with `curl`.
- [Tutorial 7](07-copilot-end-to-end.md) — govern a live Copilot
  session.
- [Tutorial 8](08-design-system-end-to-end.md) — enforce a design
  system through a governed skill.
