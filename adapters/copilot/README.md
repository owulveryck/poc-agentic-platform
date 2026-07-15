# GitHub Copilot adapter — pre-tool hook gating

This adapter wires **GitHub Copilot** (desktop app or VS Code) to the
Platform Planning Gateway with the same in-tool gating pattern used for
Claude Code. It closes the "soft half only" gap of the pre-flight adapter:
Copilot's edits are now blocked in-loop against the locked plan's
capability ticket, not just at apply time.

| Pillar | Mechanism | Component |
|---|---|---|
| 1 — Amplified planning | pre-flight (writes `.github/copilot-instructions.md`) *or* the Claude-Code MCP server (Copilot supports MCP) | [`adapters/preflight/`](../preflight/) / [`adapters/claudecode/mcpserver/`](../claudecode/mcpserver/) |
| 2 — In-tool gating | `PreToolUse` hook: JSON-in/JSON-out decision `{"permissionDecision":"deny",…}` (deny > ask > allow) | [`guard/`](guard/) |

The two are connected — as with the Claude Code adapter — by the
`.ppg-ticket` file: a successful `lock_in_plan` writes the capability
ticket there, and the guard verifies every subsequent `Edit`/`Write`
against its scope.

## Setup

1. **Start the gateway** (from the poc-agentic-platform checkout):

   ```bash
   go run ./cmd/ppg
   ```

2. **Build the guard** somewhere on your `PATH`:

   ```bash
   go build -o ~/.local/bin/ppg-copilot-guard ./adapters/copilot/guard
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
   real session id into `.ppg-session` and purges any leftover
   `.ppg-ticket`; at `PreToolUse` it verifies each `Edit`/`Write`
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
   `get_platform_guidelines_for_intent` and `lock_in_plan` under the
   standard protocol; both are usable by Copilot verbatim.

   Build it once:

   ```bash
   go build -o ~/.local/bin/ppg-mcp-server ./adapters/claudecode/mcpserver
   ```

   Then register it. The config location depends on the surface:

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
           "env": { "PPG_URL": "http://localhost:8765" },
           "tools": ["*"]
         }
       }
     }
     ```

   - **VS Code Copilot Chat** — `.vscode/mcp.json` at the workspace
     root, `servers` map, otherwise the same schema.

   `.vscode/mcp.json` is **not** picked up by the Copilot desktop app —
   don't use it there.

   Once registered, `get_platform_guidelines_for_intent` and
   `lock_in_plan` become native tool calls. On a successful lock the
   MCP server also auto-writes the capability ticket to `.ppg-ticket`
   in its cwd (the guard then picks it up).

## What happens in a session

- (Amplified path) Copilot reads `.github/copilot-instructions.md` at
  session start, or calls `get_platform_guidelines_for_intent` via MCP,
  and receives the ADR invariants for the task.
- (Locking path) Copilot proposes a structured plan; `lock_in_plan`
  either returns semantic violations or issues a ticket into
  `.ppg-ticket`.
- Every `Edit`/`Write` passes through `ppg-copilot-guard`. In scope:
  the hook emits `{"continue": true}` and the tool call proceeds.
  Out of scope, the hook returns:

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
  running inside Copilot.

## Notes and known limits

- **Payload shape**. The Copilot desktop app names the file field
  `tool_input.path`; the VS Code Copilot Chat `editFiles` tool names it
  `tool_input.file_path`. The guard accepts both.
- **Worktree model**. The Copilot desktop app runs each session in a
  git worktree of the folder you open (`cwd` is the worktree, not the
  main checkout). `.ppg-ticket` and `.ppg-session` therefore live in
  the worktree root — call `lock_in_plan` from inside the session (via
  MCP), or from a terminal opened in the worktree.
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
- **PPG_URL** overrides the gateway address for the guard when the
  smarttools helpers need to reach it (default
  `http://localhost:8000`).
