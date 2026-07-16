---
adr_id: ADR-100
title: Per-machine state directory for capability tickets and session ids
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["store", "state", "ticket", "session", "persist", "capability"]
enforcement:
  mode: programmatic
  policy_id: per_machine_state_directory
  rego: ADR-100.rego
---

## Invariant

PPG's per-project mutable state — the capability JWT and the active
session id — MUST be persisted through the `internal/store` package
(`TokenStore` and `SessionStore` interfaces). No PPG binary may read or
write those artefacts as bare files under the project's working
directory.

The default (and only PoC) implementation, `store.Filesystem`, places
them under `$XDG_STATE_HOME/ppg/projects/<slug>/` (fallback
`~/.local/state/ppg/projects/<slug>/`) with directory mode `0700` and
file mode `0600`, where `<slug>` is the base64-encoded absolute project
directory. `PPG_STORE_ROOT` and `PPG_PROJECT_DIR` (and their `--store-root`
/ `--project-dir` flags) are the sanctioned overrides.

## Rationale (durability)

Persisting session state in the project cwd is a fragility, not a
convenience:

- Different processes with different cwds (long-lived MCP server versus
  short-lived PreToolUse hook, Copilot's per-session worktree versus
  the main checkout, sandbox mode) silently miss each other's files.
- Bare files in the project are editor-visible artefacts that get
  committed by accident, require per-project `.gitignore` entries, and
  look like project data to a reader.
- The bearer-capability audit finding (`AUDIT.md`, 2026-07-10) showed
  that a cwd-scoped ticket file is trivially leaked into an adjacent
  session within the 15-min TTL.

Routing every access through an interface fixes all three: the
filesystem impl decouples storage from cwd, gives us per-project
isolation via the absolute-path slug, forces `0700`/`0600` in code, and
leaves the door open for remote implementations (HTTP, Redis, KMS-backed)
without changing any callsite.

The invariant stays true regardless of model capability — it is a
system property, not a workaround for LLM behavior. Hence AMPLIFIER, no
sunset condition.

## What we do NOT write here

We do not enumerate the interface methods (they live in
`internal/store/store.go`, canonical) and we do not mandate a specific
transport for future non-filesystem impls: only that they satisfy the
interface. Client-authentication and remote-authorization concerns for
those future impls are their own ADRs.

## Enforcement stack

- Plan-linter (this ADR's `.rego`): any plan whose steps target files
  under `adapters/` referencing the string literals `".ppg-ticket"` or
  `".ppg-session"` is rejected — the store abstraction is the only
  sanctioned path for capability persistence.
- Convention documented in `docs/reference/capability-ticket.md`: any new
  binary that participates in the amplified loop must resolve its
  `store.Filesystem` via `store.ResolveRoot` and `store.ResolveProjectDir`
  so operators can override the location consistently on both sides of
  the loop.

## Future work

- Extract a shared `internal/hookcommon/` package from the duplicated
  Claude Code and Copilot guards.
- Add non-filesystem implementations behind the same interface for
  distributed setups.
- Add server-side session-id authentication for remote store impls so
  the bearer-capability window does not reopen.
