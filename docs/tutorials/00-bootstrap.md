# Tutorial 0 — bootstrap the platform on your machine

> **Goal**: from a fresh laptop to "everything works, once" in about 10
> minutes. Every subsequent tutorial's prereqs collapse to a link back
> here.
>
> What this tutorial installs is **per-machine**, not per-project: the
> validation server, the adapter binaries, and the MCP server registration.
> Per-project wiring (hooks, contract file, skills) is covered in the
> tutorial you're running that day.
>
> Prerequisites: Go 1.25+, one of:
> - the official **GitHub Copilot desktop app** and/or `gh copilot` CLI,
> - **VS Code** + the [GitHub Copilot extensions](https://github.com/microsoft/vscode-copilot-chat),
> - the **Claude Code** CLI.
> - Python 3
> If you want to install skills (tutorials 6, 8), also
> [APM](https://microsoft.github.io/apm/quickstart/) ≥ 0.23.
>
> If you use macOS, you can install : 
> - [Go with brew](https://formulae.brew.sh/formula/go).
> - [VS Code with brew](brew install --cask visual-studio-code)

## Step 1 — Clone the platform once

You will build binaries from this checkout; keep it on disk so future
tutorials can reference it.

```bash
mkdir -p ~/src && cd ~/src
git clone https://github.com/owulveryck/poc-agentic-platform
cd poc-agentic-platform
```

## Step 2 — Build the binaries onto your `PATH`

Seven binaries, one `PATH` directory. The Makefile builds and installs
them in one shot. `~/.local/bin` is the default; override with
`BINDIR=/usr/local/bin make install` for a system-wide install.

```bash
make install
```

That produces:

- `ppg` — the validation server.
- `ppg-mcp-server` — bridges the validation server's HTTP API to agents that
  speak MCP (Claude Code, Copilot CLI/desktop, VS Code Copilot Chat).
- `ppg-guard` — PreToolUse hook for Claude Code.
- `ppg-copilot-guard` — PreToolUse hook for the GitHub Copilot surfaces.
- `ppg-preflight` — pre-flight adapter for black-box agents (see
  [tutorial 3](03-github-copilot-preflight.md)).
- `ppg-verify` — apply-time / CI control point: verifies the whole
  working-tree diff (see
  [gate changes at apply time](../how-to/gate-changes-at-apply-time.md)).
- `svc-mock` — local stand-in for a cataloged service (used by
  [tutorial 13](13-discover-a-platform-service.md)).

Verify each is on `PATH`:

```bash
which ppg ppg-mcp-server ppg-guard ppg-copilot-guard ppg-preflight
```

## Step 3 — Register the MCP server + hooks (per agent surface)

One Make target per surface writes the MCP registration AND the
`SessionStart` / `PreToolUse` hooks user-scope. Idempotent, backs up
what it modifies, never clobbers non-ppg entries. Preview with
`DRY_RUN=1`, force overwrite with `FORCE=1`.

### Claude Code

```bash
make setup-claude-code
claude mcp list        # → ppg  connected
```

Writes `mcpServers.ppg` in `~/.claude.json` and merges the ppg hooks
into `~/.claude/settings.json`.

### Copilot desktop app or `gh copilot` CLI

```bash
make setup-github-copilot
```

Writes `mcpServers.ppg` in `~/.copilot/mcp-config.json` and the
dedicated `~/.copilot/hooks/ppg.json`.

> ⚠️ **macOS GUI PATH gotcha (already handled)** — the Copilot desktop
> app is a GUI process, and macOS GUI processes do not inherit your
> shell's `PATH`. `~/.local/bin` is invisible; a bare
> `"command": "ppg-mcp-server"` silently fails with a "connecting…"
> loop. The setup script resolves the binary via `command -v` and
> writes the fully-expanded absolute path into the JSON — no manual
> path fiddling needed on your side.

### VS Code Copilot Chat

MCP config in VS Code is **workspace-scoped**, not user-scoped. It lives
in `.vscode/mcp.json` inside each project — so this is done per-project,
not here. Every downstream tutorial that targets VS Code creates that
file itself. See
[tutorial 7](07-copilot-end-to-end.md#vs-code-copilot-chat-workspace-mcp)
for the schema.

### Manual alternative (what the Make targets do)

Read `scripts/setup-claude-code.sh` and `scripts/setup-github-copilot.sh`
if you want to see the exact JSON writes, or run
`DRY_RUN=1 make setup-claude-code` to print them without touching the
disk. Rollback with `make remove-claude-code` /
`make remove-github-copilot`.

### About `PPG_PROJECT_DIR`

The MCP server keys its state (tickets, active session id) by the
absolute path of the project directory. Resolution order:

    --project-dir flag  >  PPG_PROJECT_DIR env  >  os.Getwd() at spawn

For the common case — Claude Code and Copilot desktop spawn an MCP
subprocess per session, with cwd = project root — **the `os.Getwd()`
fallback is enough and no configuration is needed**. The snippets above
omit `PPG_PROJECT_DIR` on purpose.

Set the env var explicitly only when the fallback is wrong:

- **VS Code Copilot Chat** does substitute `${workspaceFolder}` in
  `.vscode/mcp.json` — the downstream VS Code tutorials wire
  `"PPG_PROJECT_DIR": "${workspaceFolder}"` for extra safety.
- **A persistent MCP daemon** that survives project switches must be
  passed the project via `--project-dir` per invocation, because its
  spawn-time cwd is stale.

Note: Claude Code does NOT expand `${cwd}` in user-scope MCP env values
— the string is stored and passed literally. Don't put `"${cwd}"` in
`--env` there; rely on the cwd fallback instead.

## Step 4 — Start the validation server (one terminal, leave it running)

From the repo root (`-adr` is optional — [tutorial 15](15-skill-only-enforcement.md)
runs without it; `examples/` is the fictional demo corpus — point the flags
at your own directories once you have them):

```bash
ppg -addr 127.0.0.1:8765 -adr examples/adr \
    -services examples/services -service-policy examples/service-policy
```

You should see:

```
ADR store loaded: 8 invariants
Plan linter ready: 8 policies
Ticket signing key: ~/.local/state/ppg/ticket.key
Skill governance linter ready
Service catalog loaded: 4 services
Capability ticket TTL: 8h0m0s (bounded by the session)
validation server listening on :8765
```

Every downstream tutorial expects this on `:8765`. Keep this terminal
open — or run the validation server as a launchd/systemd service (see the
appendix below).

## Step 5 — Sanity-check the whole wiring

One tool call from each agent surface confirms binaries + registration
+ validation server are working end-to-end.

### From Claude Code

Same prompt in a `claude` session:

```
 Call the ppg MCP tool get_platform_guidelines_for_intent with intent
 "add an external payment provider to legacy checkout" and repository_context {"name":"bootstrap-check",
 "tech_stack":["Go"]}. Show me the JSON result.
```

### From Copilot

Open any folder in the Copilot app (or start `gh copilot`).

**Switch to Agent mode first.** Per the [GitHub Copilot MCP docs](https://docs.github.com/en/copilot/how-tos/context/model-context-protocol/extending-copilot-chat-with-mcp),
MCP tools are only invokable when the chat is in **Agent mode**
(not "Ask" or "Chat" mode). Find the mode selector near the chat
input (usually a dropdown or popup) and pick **Agent**. Without
this, `ppg` will appear in the tool list but the model will
answer *"the `get_platform_guidelines_for_intent` method doesn't
appear to be available as an installed MCP server"* — because in
non-Agent modes the tool is visible but not invokable. The
symptom is confusing on purpose (there is no error, just an
apologetic refusal).

Then chat:

> Call the ppg MCP tool `get_platform_guidelines_for_intent` with intent
> "add an external payment provider to legacy checkout" and repository_context `{"name":"bootstrap-check",
> "tech_stack":["Go"]}`. Show me the JSON result.

Expected: a JSON blob containing `architectural_invariants` with at
least ADR-042 and ADR-070. If you get "tool not found" or "not
available":
- **Are you in Agent mode?** — most likely cause (see above).
- Otherwise, recheck step 3 (absolute path in the MCP config).

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
| Build the seven binaries onto `PATH` | Enable hooks in `.github/hooks/` (Copilot) or `.claude/settings.json` (Claude) |
| Register the MCP server (user scope) | Preflight `.github/copilot-instructions.md` for the current intent |
| Start the validation server (`ppg -addr 127.0.0.1:8765 -adr …`, step 4) | (Optionally) `apm install` a skill |

## Troubleshooting

Common failure modes we've hit while shipping the tutorials:

- **`.vscode/mcp.json` is ignored by the Copilot desktop app / CLI.**
  Those surfaces read `~/.copilot/mcp-config.json`. VS Code reads
  `.vscode/mcp.json`. If your Copilot session can't find the `ppg`
  tools, check the right file for your surface.

- **Copilot lists `ppg` in the tool drawer but the model says the
  tool "isn't available" / "doesn't appear to be an installed MCP
  server".** You are almost certainly in **Ask / Chat mode**, not
  Agent mode. MCP tools are only invokable in Agent mode
  ([docs](https://docs.github.com/en/copilot/how-tos/context/model-context-protocol/extending-copilot-chat-with-mcp)).
  The mode selector is near the chat input — pick **Agent** and
  re-run the prompt. Symptom is easy to mis-diagnose: the tool
  shows up, the model even mentions it by name, but its "call"
  quietly turns into a *search* through Copilot's own built-in
  skill catalog (`agentfinder` etc.), which of course finds
  nothing PPG-related.

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

- **`PPG_URL` mismatch.** If you started the validation server on a different
  port than the MCP server was registered with, the MCP call succeeds
  but returns nothing. Re-register the MCP server with the correct
  `--env PPG_URL=…`, or restart the validation server on the expected port.

- **Startup shows fewer than `8 invariants`.** You are on a
  checkout that predates the latest ADRs (ADR-110, ADR-120). `git pull`.

## Appendix — running the validation server as a service

One command, both OSes (launchd LaunchAgent on macOS, systemd `--user`
unit on Linux); every argument after the script name is passed to `ppg`
verbatim:

```bash
scripts/setup-gateway-service.sh -adr "$PWD/examples/adr" \
    -services "$PWD/examples/services" -service-policy "$PWD/examples/service-policy"
# preview: DRY_RUN=1 scripts/setup-gateway-service.sh ...
# undo:    scripts/remove-gateway-service.sh
```

The server is holding-cost-free (no external dependencies) — you can
leave it running. Point the paths at your real corpus (not the demo
`examples/`) for a production workstation.

**✅ Done.** You have the platform installed once for every project on
this machine. From here:

- [Tutorial 1](01-first-planning-cycle.md) — the amplified loop by
  hand with `curl`.
- [Tutorial 7](07-copilot-end-to-end.md) — govern a live Copilot
  session.
- [Tutorial 8](08-design-system-end-to-end.md) — enforce a design
  system through a governed skill.
