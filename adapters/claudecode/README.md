# Claude Code adapter — MCP planning + hook gating

This adapter wires a **stock Claude Code** session to the Platform Planning
Gateway, materializing both pillars without modifying the agent:

| Pillar | Mechanism | Component |
|---|---|---|
| 1 — Amplified planning | MCP tools (`get_platform_guidelines_for_intent`, `lock_in_plan`) | [`mcpserver/`](mcpserver/) |
| 2 — In-tool gating | `PreToolUse` hook on `Edit\|Write` (exit 2 blocks, stderr goes to the model) | [`guard/`](guard/) |

The two are connected by the `.ppg-ticket` file: a successful `lock_in_plan`
writes the capability ticket there, and the guard verifies every subsequent
edit against its scope.

A fully worked, tested session is in the
[end-to-end tutorial](../../docs/tutorials/02-claude-code-end-to-end.md).

## Setup

1. **Start the gateway** (from the repository root):

   ```bash
   go run ./cmd/ppg
   ```

2. **Build the guard** somewhere on your `PATH`:

   ```bash
   go build -o ~/.local/bin/ppg-guard ./adapters/claudecode/guard
   ```

3. **Register the MCP server** in the target project:

   ```bash
   claude mcp add ppg -- go run /path/to/poc-agentic-platform/adapters/claudecode/mcpserver
   ```

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
   session id into `.ppg-session` (and purges any ticket left by a previous
   session); at `PreToolUse` it verifies each edit against the ticket.

5. **Instruct the agent** — add the contract to the target project's
   `CLAUDE.md` (see [`CLAUDE.example.md`](CLAUDE.example.md)).

## What happens in a session

- Claude calls `get_platform_guidelines_for_intent` → receives the ADR
  invariants relevant to the task.
- Claude submits its structured plan through `lock_in_plan` → either reads
  the semantic violations and corrects, or the plan locks and the ticket
  lands in `.ppg-ticket`. (The plan format is taught by the MCP tool schema,
  auto-generated from [`plan.Plan`](https://pkg.go.dev/github.com/owulveryck/poc-agentic-platform/internal/plan#Plan); the content is shaped by the invariants
  from `get_platform_guidelines_for_intent`; the behavioral rule lives in
  `CLAUDE.md`. See [How the agent knows what plan to submit](../../docs/explanation/enrichment-and-planning.md#how-the-agent-knows-what-plan-to-submit).)
- Every `Edit`/`Write` first passes through `ppg-guard`. In scope: silent.
  Out of scope, the tool call is **blocked before execution** and Claude
  reads:

  ```
  OUT_OF_PLAN_SCOPE: "internal/auth/login.go" is not part of the locked plan
  (allowed: migrations/001_seka.sql, internal/payment/router.go, ...).
  Nothing was modified. If this change is genuinely needed, re-plan through
  lock_in_plan.
  ```

  Deterministic refusal, semantic guidance, zero damage — pillar 2, running
  inside an off-the-shelf agent.

## Notes

- The guard reads `.ppg-ticket` from the hook's `cwd` (the project root).
  Add `.ppg-ticket` and `.ppg-session` to the target project's `.gitignore`.
- The ticket is bound to the session that locked the plan: the MCP server
  stamps the `.ppg-session` id into the plan at lock time, and the guard
  blocks any use of the ticket from another session (`SESSION_MISMATCH`).
  See [capability-ticket.md](../../docs/reference/capability-ticket.md).
- `PPG_URL` overrides the gateway address for the MCP server
  (default `http://localhost:8000`).
