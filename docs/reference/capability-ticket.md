# Capability ticket

An ephemeral signed JWT (HS256, symmetric secret — PoC only) issued by
`POST /lock_in_plan` on a valid plan. The Claude Code adapter writes it to
`.ppg-ticket` in the project root; the Smart Tools and the `ppg-guard` hook
verify it before acting.

| Claim | Type | Description |
|---|---|---|
| `iat` / `exp` | int | Issued at / expiry (TTL = 15 min) |
| `session_id` | string | Session the ticket is bound to; the `ppg-guard` hook rejects the ticket from any other session (`SESSION_MISMATCH`) |
| `plan_hash` | sha256 hex | Canonical fingerprint of the locked plan |
| `scope.allow_modify` | string[] | Files the agent may modify (from plan step `targets`; a prefix ending in `*` allows the subtree) |
| `scope.allow_tool` | string[] | Tools the agent may invoke (from plan step `tool` fields) |

The scope is derived mechanically from the locked plan
(`ticket.DeriveScope`): least privilege, nothing more than what the plan
declared. The ticket dies with the task (15 min TTL, no renewal endpoint)
**and with the session** (session binding, below).

## Session binding (`.ppg-session`)

The ticket is a bearer capability: whoever holds the file holds the right.
Two mechanisms bound it to the session that locked the plan:

1. At `SessionStart`, the `ppg-guard` hook writes the real Claude Code
   session id to `.ppg-session` (0600) and **purges any leftover
   `.ppg-ticket`** from a previous session.
2. At lock time, the MCP server overrides the plan's `session_id` with the
   `.ppg-session` value, so the claim carries the real session id; at every
   `Edit`/`Write`, the guard compares that claim to the `session_id` of the
   hook payload and blocks on mismatch.

When no `.ppg-session` exists (manual curl workflow, older harness), the
agent-provided `session_id` is kept and the guard skips the comparison when
the hook payload carries no session id: signature, TTL, and scope still
apply.
