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

## Deployment scope

Claude Code merges settings from four scopes (highest wins):
**Managed > CLI > Local > Project > User** — see
[Claude Code settings docs](https://code.claude.com/docs/en/settings).
Where you install the hook determines who can undo it.

| Scope | File | Writable by | Bypass by user? | Example |
|---|---|---|---|---|
| **Managed** | `/Library/Application Support/ClaudeCode/managed-settings.json` (macOS), `/etc/claude-code/managed-settings.json` (Linux), `C:\Program Files\ClaudeCode\managed-settings.json` (Windows) | root only | Not via settings — with `allowManagedHooksOnly:true`, user/project/plugin hooks are ignored entirely. The binary and environment vectors remain (see below) | [`managed-settings.example.json`](managed-settings.example.json) |
| **User** | `~/.claude/settings.json` | the user | Yes — a repo's `.claude/settings.json` overrides it, and the user can edit the file directly | [`settings.example.json`](settings.example.json) |
| **Project** | `.claude/settings.json` (committed) | anyone with repo write access | N/A — this *is* what a bypass would look like | (not used by ppg) |

For **IT-managed fleets**, install at managed scope with
`allowManagedHooksOnly:true` — this closes
[tutorial 12 A10](../../docs/tutorials/12-bypassing-the-gateway.md#a10--disable-the-guard-by-editing-its-own-config)
at the **settings layer**: the hook entries can no longer be edited,
overridden, or shadowed by the user. It does **not** by itself make the
guard tamper-proof. Two vectors remain open and must be closed
operationally:

- **The binary.** The managed hook executes whatever the command path
  points at. If `ppg-guard` lives in a user-writable location
  (`~/.local/bin`, the default `make install` target), replacing it with
  a no-op defeats the managed settings entirely. For a hostile-user
  threat model, install the binaries root-owned:
  `sudo BINDIR=/usr/local/bin make install`.
- **The environment.** The guard reads `PPG_URL` (where content checks
  are sent), `PPG_TICKET_SECRET` (the ticket signing key), and
  `PPG_STORE_ROOT` (where tickets are read) from the user's environment.
  A user can re-point verification at a rogue gateway or mint their own
  tickets. Pin these in the managed hook command for fleet deployments.

Automated via `sudo make setup-claude-code-managed` (see
[the governed-workstation how-to](../../docs/how-to/set-up-a-governed-workstation.md#a-managed-scope--recommended-for-it-managed-fleets)).

For **personal / no-root workstations**, install at user scope via
`make setup-claude-code`. The in-loop guard still refuses out-of-plan
`Edit`/`Write`, but the file itself is user-writable.

For **per-project experiments**, drop the same JSON into the repo's
`.claude/settings.json` (the shape in step 4 above works verbatim). The
end-to-end tutorial uses this scope.

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
  `PPG_PROJECT_DIR` overrides the project directory for the MCP server
  (it defaults to the process cwd, which is correct for Claude Code's
  per-project launches — set it explicitly only for daemon or long-lived
  setups); both binaries also accept `PPG_STORE_ROOT` to override the
  state root.
