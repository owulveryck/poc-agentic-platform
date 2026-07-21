# Capability ticket

An ephemeral signed JWT (HS256) issued by `POST /lock_in_plan` on a valid
plan. The signing key is `$PPG_TICKET_SECRET` when set, else a per-machine
key generated on first run at `$XDG_STATE_HOME/ppg/ticket.key` (0600) and
shared by the validation server and the guards; it is symmetric — production means
asymmetric keys behind a KMS. The Claude Code adapter persists it
through the per-machine **TokenStore** (default
`$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<session_id>`); the Smart
Tools and the `ppg-guard` hook verify it before acting.

| Claim | Type | Description |
|---|---|---|
| `iat` / `exp` | int | Issued at / expiry. The wall-clock TTL is a configurable cap (validation server `-ticket-ttl` flag / `PPG_TICKET_TTL` env, default `8h`); the session is the primary bound (see below) |
| `session_id` | string | Session the ticket is bound to; the `ppg-guard` hook rejects the ticket from any other session (`SESSION_MISMATCH`) |
| `plan_hash` | sha256 hex | Canonical fingerprint of the locked plan |
| `skill_id` | string (omitted when empty) | The skill the locked plan declared, if any; the guards, Smart Tools and `/verify_changeset` thread it back into artifact/changeset-view policy evaluation as the declared skill (other registered companions still apply by union) |
| `scope.allow_modify` | string[] | Files the agent may modify (from plan step `targets`; a prefix ending in `*` allows the subtree) |
| `scope.allow_tool` | string[] | Tools the agent may invoke (from plan step `tool` fields) |

The scope is derived mechanically from the locked plan
(`ticket.DeriveScope`): least privilege, nothing more than what the plan
declared. The ticket dies **with the session** (session binding, below —
the primary bound) and, as a defense-in-depth cap, at its wall-clock TTL
(configurable via `-ticket-ttl` / `PPG_TICKET_TTL`, default `8h`; there is
no renewal endpoint, so set it to cover your longest expected session).

## Storage layout

The ticket and its companion "active session id" live under a per-machine
state root (default `$XDG_STATE_HOME/ppg`, fallback `~/.local/state/ppg`),
scoped to the project's absolute path:

```
$STORE_ROOT/projects/<slug>/session          # active session id (0600)
$STORE_ROOT/projects/<slug>/tickets/<sid>    # JWT for that session (0600)
$STORE_ROOT/projects/<slug>/.lock            # per-project advisory lock (0600)
```

`<slug>` is a `base64.RawURLEncoding` of the project's absolute path
(symlinks resolved). Directories are `0700`, files `0600`.

Store operations are safe under concurrent processes (the guards, the MCP
server and `ppg-verify` are independent short-lived processes that can touch
one project at once). Writes use a same-directory temp file + atomic
`rename(2)`, and every operation takes a per-project advisory `flock` on
`.lock` — exclusive for writers, shared for readers — so a `Reset` at
SessionStart cannot interleave with a concurrent `Put` (no half-purged state,
no torn reads). The lock is released by the OS when the process exits, so a
crash never leaves it stale.

- Override the store root with `--store-root` or `PPG_STORE_ROOT` on any
  of the three binaries (`ppg-mcp-server`, `ppg-guard`,
  `ppg-copilot-guard`).
- Override the project directory with `--project-dir` or `PPG_PROJECT_DIR`.
  The MCP server falls back to `os.Getwd()` at spawn (fine for Claude
  Code / Copilot desktop, which spawn a fresh subprocess per session);
  the two guards additionally consider the hook payload's `cwd`.

## Session binding

The ticket is a bearer capability: whoever can read the file holds the
right. Two mechanisms bind it to the session that locked the plan:

1. At `SessionStart`, the `ppg-guard` hook calls `SessionStore.PutActive`
   with the real Claude Code session id and `TokenStore.Reset` — every
   ticket for this project is purged, so a leftover capability never
   survives the session that locked it.
2. At lock time, the MCP server reads `SessionStore.GetActive` and
   overrides the plan's `session_id` with it before signing. At every
   `Edit`/`Write`, the guard compares the ticket's `session_id` claim to
   the `session_id` of the hook payload and blocks on mismatch.

When no active session is recorded (manual curl workflow, older harness),
the agent-provided `session_id` is kept and the guard skips the comparison
when the hook payload carries no session id: signature, TTL, and scope
still apply.

## Known limitation: Copilot desktop user-scope MCP config

Copilot desktop's `~/.copilot/mcp-config.json` is user-scoped and does not
expand `${workspaceFolder}` — so `PPG_PROJECT_DIR` cannot be set once for
all projects there. Workaround: launch the MCP server from a per-project
shell wrapper that exports `PPG_PROJECT_DIR`, or accept a single shared
slug for every project on the machine (which defeats project isolation).
A future fix will pass the project id via the `lock_in_plan` tool
arguments so the MCP server can resolve it per-call.
