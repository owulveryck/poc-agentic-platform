# Tutorial — govern a live GitHub Copilot session

> **Goal**: wire the official GitHub Copilot desktop app (or the
> `gh copilot` CLI / VS Code Copilot Chat) to the gateway and watch both
> pillars work end to end: the plan is enriched and locked through MCP
> tools, and an out-of-plan edit is blocked by a hook *before* it executes.
>
> Time: ~20 minutes. Prerequisites: Go 1.25+, the official GitHub Copilot
> app installed and signed in, this repository cloned (the paths below
> assume `/path/to/poc-agentic-platform`).
>
> This tutorial is the Copilot sibling of
> [tutorial 2](02-claude-code-end-to-end.md). If you have read that one,
> only steps 4 (MCP registration) and 6 (contract file) genuinely differ.

## Step 1 — Start the gateway

From the `poc-agentic-platform` root:

```bash
go run ./cmd/ppg -addr :8765
```

Wait for `Platform Planning Gateway listening on :8765`. Keep this
terminal open.

## Step 2 — Create a scratch target project

The governed session runs in a *separate* project, like any team repo:

```bash
mkdir ~/ppg-copilot-demo && cd ~/ppg-copilot-demo && git init
printf '.ppg-ticket\n.ppg-session\n' >> .gitignore
mkdir -p internal/payment internal/auth
printf 'package payment\n' > internal/payment/router.go
printf 'package auth\n'    > internal/auth/login.go
git add -A && git commit -q -m "init"
```

`internal/auth/` is one of the frozen legacy paths of ADR-070 — we will
use it to trigger a refusal in step 8.

## Step 3 — Build and install the binaries

Two binaries: the pre-tool guard, and the MCP server that exposes the
gateway's planning endpoints to Copilot.

```bash
go build -o ~/.local/bin/ppg-copilot-guard /path/to/poc-agentic-platform/adapters/copilot/guard
go build -o ~/.local/bin/ppg-mcp-server    /path/to/poc-agentic-platform/adapters/claudecode/mcpserver
```

(`~/.local/bin` must be on your `PATH`.) The MCP server is the same
protocol-standard binary the Claude Code adapter uses — Copilot speaks
the same protocol, no fork needed.

## Step 4 — Register the MCP server (Copilot-specific)

The MCP config location depends on which Copilot surface you use:

- **Official Copilot desktop app / `gh copilot` CLI** —
  `~/.copilot/mcp-config.json`, or use the equivalent shortcut:

  ```bash
  copilot mcp add ppg --env PPG_URL=http://localhost:8765 -- ppg-mcp-server
  ```

  Or edit the file directly:

  ```json
  {
    "mcpServers": {
      "ppg": {
        "type": "stdio",
        "command": "ppg-mcp-server",
        "env": { "PPG_URL": "http://localhost:8765" },
        "tools": ["*"]
      }
    }
  }
  ```

- **VS Code Copilot Chat** — `.vscode/mcp.json` at the workspace root
  (top-level `servers` map, otherwise the same schema).

Note: `.vscode/mcp.json` is **not** picked up by the desktop app or the
`gh copilot` CLI. Wire the correct file for your surface — a
misregistered server silently fails, and Copilot falls back to
hand-rolling stdio calls.

**What you should observe**: after registering, `get_platform_guidelines_for_intent`
and `lock_in_plan` appear in Copilot's tool list (Command Palette →
"MCP: List Servers" in VS Code; the desktop app surfaces MCP tools in
its tools drawer).

## Step 5 — Register the hooks

Create `~/ppg-copilot-demo/.github/hooks/ppg.json` — copy from
[`adapters/copilot/settings.example.json`](../../adapters/copilot/settings.example.json):

```json
{
  "hooks": {
    "SessionStart": [
      { "type": "command", "command": "ppg-copilot-guard", "timeoutSec": 5 }
    ],
    "PreToolUse": [
      { "type": "command", "command": "ppg-copilot-guard", "timeoutSec": 5 }
    ]
  }
}
```

The `SessionStart` entry binds tickets to sessions: it records the
session id in `.ppg-session` (which the MCP server stamps into the plan
at lock time) and purges any ticket left by a previous session. The
`PreToolUse` entry gates every `Edit` / `Write` against the ticket
scope.

## Step 6 — Add the behavioral contract (Copilot-specific)

The Copilot equivalent of `CLAUDE.md` is
`.github/copilot-instructions.md`. Seed it with the ADR invariants for
the current intent via the pre-flight adapter, then append the contract:

```bash
# In ~/ppg-copilot-demo
PPG_URL=http://localhost:8765 \
  go run /path/to/poc-agentic-platform/adapters/preflight \
  -repo ppg-copilot-demo -stack Go,SQL \
  "Add the Seka payment method to checkout"

cat >> .github/copilot-instructions.md <<'EOF'

# Platform contract

- Before planning any change, call `get_platform_guidelines_for_intent` with
  the intent and the repository context: the platform returns the
  architectural invariants (ADRs) your plan must honor.
- Before modifying anything, submit your structured plan through
  `lock_in_plan`. No edit is accepted without a locked plan: a `PreToolUse`
  hook verifies every file against the capability ticket and blocks anything
  outside the locked scope.
- If a tool refuses with `OUT_OF_PLAN_SCOPE`, do not retry the same call:
  either stay within the locked plan, or re-plan through `lock_in_plan` if
  the extra change is genuinely needed.
- In the plan you lock, make platform-relevant steps explicit: a test step
  whose action runs `go test`, a migration step targeting `migrations/`.
  Violation messages name the exact criterion; fix precisely that.
EOF
```

The pre-flight writes the ADR invariants at the top; the appended
contract binds the workflow to the MCP tools you registered in step 4.

## Step 7 — Run the governed session

Open `~/ppg-copilot-demo` as a folder in the Copilot app and prompt:

> Add the Seka payment method to checkout

**What you should observe**, in order:

1. Copilot calls `get_platform_guidelines_for_intent` and receives
   ADR-042 (egress proxy) and ADR-070 (frozen paths) — the same payload
   you saw in [tutorial 1, step 2](01-first-planning-cycle.md).
2. Copilot drafts a plan and submits it through `lock_in_plan`. If the
   plan misses a `go test` step, the gateway answers `PLAN_REJECTED`
   with the `go_tests_present` violation and Copilot self-corrects in
   one round-trip.

   *Troubleshooting*: if a plan is rejected repeatedly with the same
   violation, the message names the exact criterion the policy checks
   (a step whose tool is `go-test` or whose action runs `go test`). A
   rejection loop means the plan does not satisfy that criterion,
   however plausible its steps look; do not guess along other
   dimensions.
3. On success the response is `PLAN_LOCKED`, and the ticket lands in
   `.ppg-ticket` (auto-written by the MCP server). Inspect its claims:

   ```bash
   python3 -c "import base64,json; p=open('.ppg-ticket').read().strip().split('.')[1]; \
   print(json.dumps(json.loads(base64.urlsafe_b64decode(p+'='*(-len(p)%4))), indent=2))"
   ```

4. Every `Edit` / `Write` inside the locked scope passes silently
   through `ppg-copilot-guard`.

## Step 8 — Trigger the drift refusal

In the same session, prompt:

> Also quickly update internal/auth/login.go

**What you should observe**: the hook denies the edit *before execution*
with a JSON `permissionDecision: "deny"`, and Copilot surfaces the
reason to you:

```
OUT_OF_PLAN_SCOPE: "internal/auth/login.go" is not part of the locked
plan (allowed: migrations/001_seka.sql, internal/payment/router.go,
tests/integration_payment_test.go). Nothing was modified. If this
change is genuinely needed, re-plan through lock_in_plan.
```

Per the contract in `.github/copilot-instructions.md`, Copilot does not
retry the same call: it either stays within the plan or re-plans
through `lock_in_plan`. If no plan is locked at all, the guard blocks
with `No capability ticket found (.ppg-ticket)` and points to
`lock_in_plan` — the paved road is also the only road.

One more property to observe: quit and start a **new** Copilot session
in the same directory. The `SessionStart` hook purges the previous
ticket, and even a copy of it would be refused (`SESSION_MISMATCH`:
the ticket's `session_id` claim no longer matches the session). A
capability dies with the session that locked it, not only with its
15-minute TTL.

## Step 9 — Copilot-specific notes

- **Git-worktree model**. The Copilot desktop app runs each session in
  a git worktree of the folder you open (`cwd` is the worktree, not
  the main checkout). `.ppg-ticket` and `.ppg-session` therefore live
  in the worktree root, and the MCP server / hook resolve everything
  relative to it — no configuration needed.
- **Hooks discovery**. `.github/hooks/*.json` is the native GitHub
  location; `.claude/settings.json` also works (Claude-compatible).
  Workspace hooks take precedence over user hooks.
- **Hooks are Preview**. Format and behavior may change; watch
  [the hooks reference](https://code.visualstudio.com/docs/agents/reference/hooks-reference).
- **What this adapter does NOT gate**. `ppg-copilot-guard` handles
  `Edit` / `Write` / `editFiles`. It leaves `Read` / `Glob` /
  `runTerminalCommand` alone — for those, use the
  `chat.agent.sandbox.fileSystem.*` and `chat.tools.terminal.autoApprove`
  settings.

## Step 10 — Clean up

```bash
copilot mcp remove ppg   # or delete the entry from ~/.copilot/mcp-config.json
rm -rf ~/ppg-copilot-demo
```

**✅ Done.** You have seen pillar 1 (amplified planning via MCP) and
pillar 2 (deterministic in-tool gating via the hook) run inside the
official GitHub Copilot app. The *why* is in
[capability-tickets-and-in-tool-guards.md](../explanation/capability-tickets-and-in-tool-guards.md).
Next step: package this workflow as a governed skill and watch it drive
the session by itself, in [tutorial 6](06-skill-to-session-end-to-end.md)
(applies verbatim; the skill's body is agent-neutral).
