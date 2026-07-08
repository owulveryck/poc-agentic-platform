# Capability ticket

An ephemeral signed JWT (HS256, symmetric secret — PoC only) issued by
`POST /lock_in_plan` on a valid plan. The Claude Code adapter writes it to
`.ppg-ticket` in the project root; the Smart Tools and the `ppg-guard` hook
verify it before acting.

| Claim | Type | Description |
|---|---|---|
| `iat` / `exp` | int | Issued at / expiry (TTL = 15 min) |
| `session_id` | string | Originating session |
| `plan_hash` | sha256 hex | Canonical fingerprint of the locked plan |
| `scope.allow_modify` | string[] | Files the agent may modify (from plan step `targets`; a prefix ending in `*` allows the subtree) |
| `scope.allow_tool` | string[] | Tools the agent may invoke (from plan step `tool` fields) |

The scope is derived mechanically from the locked plan
(`ticket.DeriveScope`): least privilege, nothing more than what the plan
declared. The ticket dies with the task (15 min TTL, no renewal endpoint).
