# GitHub Copilot adapter — pre-tool hook gating

This adapter wires **GitHub Copilot** (desktop app or VS Code) to the
Platform Planning Gateway with the same in-tool gating pattern used for
Claude Code. It closes the "soft half only" gap of the pre-flight adapter:
Copilot's edits are now blocked in-loop against the locked plan — both the
**file scope** (the capability ticket) and the **content** (the
artifact-view policy corpus, via the gateway's `/verify_artifact`), not
just at apply time.

| Pillar | Mechanism | Component |
|---|---|---|
| 1 — Amplified planning | pre-flight (writes `.github/copilot-instructions.md`) *or* the Claude-Code MCP server (Copilot supports MCP) | [`adapters/preflight/`](../preflight/) / [`adapters/claudecode/mcpserver/`](../claudecode/mcpserver/) |
| 2 — In-tool gating | `PreToolUse` hook: JSON-in/JSON-out decision `{"permissionDecision":"deny",…}` (deny > ask > allow); gates path scope **and** content | [`guard/`](guard/) |

The two are connected — as with the Claude Code adapter — by the
per-machine **TokenStore** (default
`$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<session_id>`): a
successful `lock_in_plan` persists the capability ticket there, and the
guard reads it back to verify every subsequent `Edit`/`Write` against
its scope.

## Setup

1. **Install all binaries** (from the poc-agentic-platform checkout —
   one command builds and installs into `~/.local/bin`):

   ```bash
   make install
   ```

2. **Start the gateway** (`-adr` is required; point it at your ADR
   corpus — here the fictional demo corpus, from the checkout root):

   ```bash
   ppg -addr :8765 -adr examples/adr
   ```

3. **Register the hook** in the target project — copy
   [`settings.example.json`](settings.example.json) to
   `.github/hooks/ppg.json`:

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

   The same binary serves both events: at `SessionStart` it records the
   real session id via the SessionStore and purges any leftover tickets
   from the TokenStore; at `PreToolUse` it verifies each `Edit`/`Write`
   against the ticket.

4. **(Optional) Amplify with invariants** — run the pre-flight to seed
   `.github/copilot-instructions.md` for the current intent:

   ```bash
   PPG_URL=http://localhost:8765 ppg-preflight \
     -repo <your-repo-name> -stack Go,SQL \
     "Add the Seka payment method to checkout"
   ```

5. **(Optional) Add the MCP planning tools** — Copilot supports MCP
   servers. The Claude-Code MCP server exposes
   `get_platform_guidelines_for_intent`, `find_platform_service` and
   `lock_in_plan` under the standard protocol; all three are usable by
   Copilot verbatim.

   `ppg-mcp-server` was already installed by `make install` in step 1.
   Register it. The config location depends on the surface:

   - **Copilot CLI / desktop app** — `~/.copilot/mcp-config.json`
     (`mcpServers` map, `type: "local"` or `"stdio"`), or the
     equivalent `copilot mcp add` shortcut:

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
           "env": {
             "PPG_URL": "http://localhost:8765",
             "PPG_PROJECT_DIR": "/abs/path/to/project"
           },
           "tools": ["*"]
         }
       }
     }
     ```

     `PPG_PROJECT_DIR` is required: the MCP server is long-lived, its
     cwd is stale as soon as you switch project. Copilot desktop's
     user-scope config does not expand `${workspaceFolder}` — see the
     workaround note in
     [capability-ticket.md](../../docs/reference/capability-ticket.md#known-limitation-copilot-desktop-user-scope-mcp-config).

   - **VS Code Copilot Chat** — `.vscode/mcp.json` at the workspace
     root, `servers` map, otherwise the same schema.

   `.vscode/mcp.json` is **not** picked up by the Copilot desktop app —
   don't use it there.

   Once registered, `get_platform_guidelines_for_intent`,
   `find_platform_service` and `lock_in_plan` become native tool
   calls. On a successful lock the
   MCP server persists the capability ticket through the per-machine
   TokenStore (the guard then reads it back via the same store).

## What happens in a session

- (Amplified path) Copilot reads `.github/copilot-instructions.md` at
  session start, or calls `get_platform_guidelines_for_intent` via MCP,
  and receives the ADR invariants for the task.
- (Locking path) Copilot proposes a structured plan; `lock_in_plan`
  either returns semantic violations or persists a ticket through the
  TokenStore.
- Every `Edit`/`Write` passes through `ppg-copilot-guard`. It first
  checks the target path against the ticket scope, then — when the path
  is allowed — sends the edited content to the gateway's
  `/verify_artifact` for the content policy. In scope and clean: the
  hook emits `{"continue": true}` and the tool call proceeds. Out of
  scope, the hook returns:

  ```json
  {
    "hookSpecificOutput": {
      "hookEventName": "PreToolUse",
      "permissionDecision": "deny",
      "permissionDecisionReason": "OUT_OF_PLAN_SCOPE: \"internal/auth/login.go\" is not part of the locked plan (allowed: migrations/001_seka.sql, internal/payment/router.go, ...). Nothing was modified. If this change is genuinely needed, re-plan through lock_in_plan."
    }
  }
  ```

  Copilot surfaces the reason to the user and stops the edit.
  Deterministic refusal, semantic guidance, zero damage — pillar 2,
  running inside Copilot. A path that is in scope but whose content
  breaks an invariant is denied the same way, with reason prefixed
  `ARCHITECTURAL_INVARIANT_VIOLATION` (the messages from
  `/verify_artifact`). The guard **fails closed**: if it cannot evaluate
  an edit (unreadable payload, unopenable store, unreachable gateway) it
  denies with `PPG_GUARD_ERROR: … (fail-closed)` rather than letting the
  edit through.

## Content policy and composability

`ppg-copilot-guard` enforces **both** path scope (ticket-driven) and
content: after the path check it sends the edited bytes to the gateway's
`/verify_artifact`, which runs the artifact-view Rego corpus. A content
invariant is therefore authored once as an ADR's `.rego` (see
[docs/how-to/enforce-a-content-invariant.md](../../docs/how-to/enforce-a-content-invariant.md))
and enforced by this same guard — no separate hook to ship. The
`design-system` skill in
[`demo/skills/design-system/`](../../demo/skills/design-system/) relies on
exactly this: its rules live in `examples/adr/ADR-090.rego` (artifact altitude),
not in a bespoke script.

If you still need a check that cannot be expressed against the corpus,
multiple hooks per `PreToolUse` event compose: the runtime fires them in
parallel and applies the most-restrictive decision (`deny` > `ask` >
`allow`), so a standalone content-scope hook can run alongside this one.

## Notes and known limits

- **Payload shape**. The Copilot desktop app names the file field
  `tool_input.path`; the VS Code Copilot Chat `editFiles` tool names it
  `tool_input.file_path`. The guard accepts both.
- **Worktree model**. The Copilot desktop app runs each session in a
  git worktree of the folder you open (`cwd` is the worktree, not the
  main checkout). The TokenStore keys tickets on the absolute path of
  the project (base64-encoded), so each worktree gets its own slug
  under `$XDG_STATE_HOME/ppg/projects/` and there is no cross-worktree
  contamination.
- **Config discovery**. VS Code / Copilot look for hooks in
  `.github/hooks/*.json` (workspace, native GitHub location) or
  `.claude/settings.json` (workspace, Claude-compatible). Workspace
  hooks take precedence over user hooks; see
  [the hooks reference](https://code.visualstudio.com/docs/agents/reference/hooks-reference).
- **Preview surface**. Copilot agent hooks are marked *Preview* — the
  format may change; watch the docs.
- **What this guard does NOT cover**. It gates `Edit` / `Write` /
  `editFiles`. It does not gate `Read` / `Glob` / `Bash` /
  `runTerminalCommand` — those need a separate policy (e.g., the
  `chat.agent.sandbox.fileSystem.*` and
  `chat.tools.terminal.autoApprove` settings). Read gating is out of
  scope for a plan-scope guard.
- **PPG_URL** is the gateway base URL the guard POSTs the edited content
  to for the `/verify_artifact` content check (default
  `http://localhost:8765`) — the same convention as the MCP server.
