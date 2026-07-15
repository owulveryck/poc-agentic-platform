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

Writes `~/.copilot/mcp-config.json`:

```bash
copilot mcp add ppg --env PPG_URL=http://localhost:8765 -- ppg-mcp-server
copilot mcp list       # → ppg  connected
```

### Claude Code

```bash
claude mcp add ppg --scope user --env PPG_URL=http://localhost:8765 -- ppg-mcp-server
claude mcp list        # → ppg  connected
```

### VS Code Copilot Chat

MCP config in VS Code is **workspace-scoped**, not user-scoped. It lives
in `.vscode/mcp.json` inside each project — so this is done per-project,
not here. Every downstream tutorial that targets VS Code creates that
file itself. See
[tutorial 7](07-copilot-end-to-end.md#step-4--register-the-mcp-server-copilot-specific)
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
- **`ppg-copilot-guard: command not found` inside a hook.** The
  binary is not on the `PATH` the Copilot desktop app sees. Either
  install it to a system location (`/usr/local/bin`), or make the
  hook config path absolute.
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
