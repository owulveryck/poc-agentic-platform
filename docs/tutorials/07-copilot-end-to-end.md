# Tutorial — govern a live GitHub Copilot session

> **Goal**: wire the official GitHub Copilot desktop app (or the
> `gh copilot` CLI / VS Code Copilot Chat) to the gateway and watch both
> pillars work end to end: the plan is enriched and locked through MCP
> tools, and an out-of-plan edit is blocked by a hook *before* it executes.
>
> Time: ~10 minutes. Prerequisites: [tutorial 0](00-bootstrap.md) completed
> (gateway running on `:8765`; `ppg-copilot-guard` and `ppg-mcp-server` on
> `PATH`; `copilot mcp list` shows `ppg` as connected — or for VS Code
> readers, see the note below).
>
> This tutorial is the Copilot sibling of
> [tutorial 2](02-claude-code-end-to-end.md). Only step 3 (Copilot's
> equivalent of `CLAUDE.md`) genuinely differs.

### VS Code Copilot Chat: workspace MCP

Skip this section if you use the Copilot desktop app or `gh copilot`
CLI (tutorial 0 already registered `ppg` user-scope for those).

VS Code reads MCP config from the workspace, not the user profile.
After step 1 below, add `.vscode/mcp.json` to `~/ppg-copilot-demo`:

```json
{
  "servers": {
    "ppg": {
      "type": "stdio",
      "command": "ppg-mcp-server",
      "env": { "PPG_URL": "http://localhost:8765" }
    }
  }
}
```

`.vscode/mcp.json` is not picked up by the desktop app or the `gh
copilot` CLI. Wire the correct file for your surface — a misregistered
server silently fails, and Copilot falls back to hand-rolling stdio
calls.

## Step 1 — Create a scratch target project

The governed session runs in a *separate* project, like any team repo:

```bash
mkdir ~/ppg-copilot-demo && cd ~/ppg-copilot-demo && git init
mkdir -p internal/payment internal/auth
printf 'package payment\n' > internal/payment/router.go
printf 'package auth\n'    > internal/auth/login.go
git add -A && git commit -q -m "init"
```

Session state (active session id + capability ticket) is persisted under
`$XDG_STATE_HOME/ppg/projects/<slug>/` outside the project — nothing to
add to `.gitignore`.

`internal/auth/` is one of the frozen legacy paths of ADR-070 — we will
use it to trigger a refusal in step 5.

## Step 2 — Register the hooks

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
session id via the SessionStore (which the MCP server reads at lock
time to stamp the plan) and purges any ticket left by a previous
session. Both live under `$XDG_STATE_HOME/ppg/projects/<slug>/` outside
the project. The `PreToolUse` entry gates every `Edit` / `Write`
against the ticket scope.

## Step 3 — Add the behavioral contract (Copilot-specific)

The Copilot equivalent of `CLAUDE.md` is
`.github/copilot-instructions.md`. Seed it with the ADR invariants for
the current intent via the pre-flight adapter (`ppg-preflight`,
installed by `make install` — see
[tutorial 0](00-bootstrap.md#step-2--build-the-binaries-onto-your-path)),
then append the contract:

```bash
# In ~/ppg-copilot-demo
PPG_URL=http://localhost:8765 ppg-preflight \
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
contract binds the workflow to the `ppg` MCP tools registered in
[tutorial 0](00-bootstrap.md).

## Step 4 — Run the governed session

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
3. On success the response is `PLAN_LOCKED`, and the ticket is persisted
   through the per-machine TokenStore at
   `$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<sid>` (auto-written by
   the MCP server). Inspect its claims (adjust `SID` to the session id
   Copilot reports):

   ```bash
   SLUG=$(printf '%s' "$PWD" | base64 | tr '+/' '-_' | tr -d '=')
   SID=<your-session-id>
   TICKET="${XDG_STATE_HOME:-$HOME/.local/state}/ppg/projects/$SLUG/tickets/$SID"
   python3 -c "import base64,json; p=open('$TICKET').read().strip().split('.')[1]; \
   print(json.dumps(json.loads(base64.urlsafe_b64decode(p+'='*(-len(p)%4))), indent=2))"
   ```

4. Every `Edit` / `Write` inside the locked scope passes silently
   through `ppg-copilot-guard`.

## Step 5 — Trigger the drift refusal

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
with `No capability ticket for this session` and points to
`lock_in_plan` — the paved road is also the only road.

One more property to observe: quit and start a **new** Copilot session
in the same directory. The `SessionStart` hook purges the previous
ticket, and even a copy of it would be refused (`SESSION_MISMATCH`:
the ticket's `session_id` claim no longer matches the session). A
capability dies with the session that locked it, not only with its
configurable wall-clock TTL.

## Step 6 — Copilot-specific notes

- **Git-worktree model**. The Copilot desktop app runs each session in
  a git worktree of the folder you open (`cwd` is the worktree, not
  the main checkout). Because the TokenStore keys tickets on the
  absolute project path (`base64` of the worktree root), each worktree
  gets its own slug under `$XDG_STATE_HOME/ppg/projects/` — no
  cross-contamination between sessions.
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

## Step 7 — Clean up

```bash
rm -rf ~/ppg-copilot-demo
```

(The `ppg` MCP registration is user-scope from [tutorial 0](00-bootstrap.md);
leave it in place for the next tutorial. Remove it with
`copilot mcp remove ppg` — or delete the entry from
`~/.copilot/mcp-config.json` — if you are unwinding the whole setup.)

**✅ Done.** You have seen pillar 1 (amplified planning via MCP) and
pillar 2 (deterministic in-tool gating via the hook) run inside the
official GitHub Copilot app. The *why* is in
[capability-tickets-and-in-tool-guards.md](../explanation/capability-tickets-and-in-tool-guards.md).
Next step: package this workflow as a governed skill and watch it drive
the session by itself, in [tutorial 6](06-skill-to-session-end-to-end.md)
(applies verbatim; the skill's body is agent-neutral).
