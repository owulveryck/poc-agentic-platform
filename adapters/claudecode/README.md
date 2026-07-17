# Claude Code adapter — MCP planning + hook gating

This adapter wires a **stock Claude Code** session to the Platform Planning
Gateway, materializing both pillars without modifying the agent:

| Pillar | Mechanism | Component |
|---|---|---|
| 1 — Amplified planning | MCP tools (`get_platform_guidelines_for_intent`, `find_platform_service`, `lock_in_plan`) | [`mcpserver/`](mcpserver/) |
| 2 — In-tool gating | `PreToolUse` hook on `Edit\|Write` (exit 2 blocks, stderr goes to the model); gates path scope **and** content (via the gateway's `/verify_artifact`) | [`guard/`](guard/) |

The two are connected by the per-machine **TokenStore** (default
`$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<session_id>`): a successful
`lock_in_plan` persists the capability ticket there, and the guard reads
it back to verify every subsequent edit against its scope. See
[capability-ticket.md — Storage layout](../../docs/reference/capability-ticket.md#storage-layout).

A fully worked, tested session is in the
[end-to-end tutorial](../../docs/tutorials/02-claude-code-end-to-end.md).

## Setup

1. **Install all binaries** (from the repository root — one command
   builds all seven: `ppg`, `ppg-mcp-server`, `ppg-guard`,
   `ppg-copilot-guard`, `ppg-preflight`, `ppg-verify`, `svc-mock`
   into `~/.local/bin`):

   ```bash
   make install
   ```

2. **Start the gateway** (`-adr` is required; point it at your ADR
   corpus — here the fictional demo corpus, from the repository root):

   ```bash
   ppg -addr :8765 -adr examples/adr
   ```

3. **Register the MCP server** in the target project:

   ```bash
   claude mcp add ppg -- ppg-mcp-server
   ```

   The MCP server's project dir defaults to its cwd at spawn — reliable
   in Claude Code, which spawns a fresh subprocess per session. Set
   `--env PPG_PROJECT_DIR=/abs/path` explicitly only for daemon setups.

4. **Register the hooks** — merge [`settings.example.json`](settings.example.json)
   into the target project's `.claude/settings.json`:

   ```json
   {
     "hooks": {
       "SessionStart": [
         {
           "hooks": [
             { "type": "command", "command": "ppg-guard", "args": [] }
           ]
         }
       ],
       "PreToolUse": [
         {
           "matcher": "Edit|Write",
           "hooks": [
             { "type": "command", "command": "ppg-guard", "args": [] }
           ]
         }
       ]
     }
   }
   ```

   The same binary serves both events: at `SessionStart` it records the
   session id via the SessionStore and purges any ticket left by a previous
   session; at `PreToolUse` it verifies each edit against the ticket.

5. **Instruct the agent** — add the contract to the target project's
   `CLAUDE.md` (see [`CLAUDE.example.md`](CLAUDE.example.md)).

## What happens in a session

- Claude calls `get_platform_guidelines_for_intent` → receives the ADR
  invariants relevant to the task.
- Claude submits its structured plan through `lock_in_plan` → either reads
  the semantic violations and corrects, or the plan locks and the ticket is
  persisted through the TokenStore. (The plan format is taught by the MCP
  tool schema,
  auto-generated from [`plan.Plan`](https://pkg.go.dev/github.com/owulveryck/poc-agentic-platform/internal/plan#Plan); the content is shaped by the invariants
  from `get_platform_guidelines_for_intent`; the behavioral rule lives in
  `CLAUDE.md`. See [How the agent knows what plan to submit](../../docs/explanation/enrichment-and-planning.md#how-the-agent-knows-what-plan-to-submit).)
- Every `Edit`/`Write` first passes through `ppg-guard`. It checks the
  target path against the ticket scope, then — when the path is allowed —
  sends the edited content to the gateway's `/verify_artifact` for the
  content policy. In scope and clean: silent. Out of scope, the tool call
  is **blocked before execution** and Claude reads:

  ```
  OUT_OF_PLAN_SCOPE: "internal/auth/login.go" is not part of the locked plan
  (allowed: migrations/001_seka.sql, internal/payment/router.go, ...).
  Nothing was modified. If this change is genuinely needed, re-plan through
  lock_in_plan.
  ```

  A path that is in scope but whose content breaks an invariant is blocked
  the same way, prefixed `ARCHITECTURAL_INVARIANT_VIOLATION`. The guard
  **fails closed**: if it cannot evaluate an edit (unreadable payload,
  unopenable store, unreachable gateway) it blocks with `PPG_GUARD_ERROR:
  … (fail-closed)` rather than letting it through.

  Deterministic refusal, semantic guidance, zero damage — pillar 2, running
  inside an off-the-shelf agent.

## Notes

- Session state (active session id + tickets keyed by session id) lives
  under `$XDG_STATE_HOME/ppg/projects/<slug>/`. Nothing is written inside
  the project — no `.gitignore` edits needed.
- The ticket is bound to the session that locked the plan: the MCP server
  reads the active session id from the SessionStore at lock time and
  stamps it into the plan, and the guard blocks any use of the ticket
  from another session (`SESSION_MISMATCH`). See
  [capability-ticket.md](../../docs/reference/capability-ticket.md).
- `PPG_URL` overrides the gateway address for both the MCP server and
  `ppg-guard` — the guard POSTs edited content to `/verify_artifact` for
  the content check (default `http://localhost:8765`).
  `PPG_PROJECT_DIR` is required for the MCP server; both binaries also
  accept `PPG_STORE_ROOT` to override the state root.
