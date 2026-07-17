# Gateway and adapter binaries

## `cmd/ppg` — the gateway

```bash
ppg [flags]
```

Install once with `make install` from the repo root (`~/.local/bin/ppg`
by default; override with `BINDIR`).

| Flag | Default | Description |
|---|---|---|
| `-addr` | `:8765` | Listen address |
| `-adr` | *(none — required)* | Path to the ADR store (Markdown + paired `.rego` files). Startup fails without it, and fails if the directory contains no `*.md`. The fictional demo corpus is `examples/adr` |
| `-skill-governance` | `skill-governance` | Path to the skill governance Rego policy directory |
| `-skills` | *(none — Gate 3 off)* | Path to the published skills directory (one subdir per skill with `SKILL.md` [+ `SKILL.rego`]). Enables Gate 3: a plan declaring `skill_id` is additionally linted against that skill's companion Rego; with the flag unset, any `skill_id` is rejected (`unknown_skill`, fail closed) |
| `-services` | *(none — catalog disabled)* | Path to the service catalog (`*.md` records). Omitted: `/discover_service` answers `SERVICE_CATALOG_UNAVAILABLE` |
| `-service-policy` | *(none)* | Path to the service-catalog ranking Rego policy directory. Requires `-services`; a policy that fails to load is a startup error |
| `-ticket-ttl` | `0` | Capability ticket wall-clock lifetime (a Go duration, e.g. `8h`, `30m`). `0` means use `$PPG_TICKET_TTL`, else the built-in default `8h`. The session still bounds the ticket regardless. |
| `-allow-wide-scope` | `false` | Accept plan targets like `.` or `*` whose derived ticket would be allow-all. Off by default: the built-in `scope_breadth_cap` rejects them at lock time |
| `-version` | `false` | Print the version and exit (all seven binaries accept it) |

The ticket lifetime resolves as `-ticket-ttl` (when > 0) > `$PPG_TICKET_TTL`
> built-in `8h`; a malformed `PPG_TICKET_TTL` is a startup error. Startup logs
the resolved TTL.

The ticket signing key resolves as `$PPG_TICKET_SECRET` (used verbatim) >
the per-machine key file `$XDG_STATE_HOME/ppg/ticket.key` (hex, generated
0600 on first run). The guards load the same key to verify tickets locally.

The HTTP server bounds request bodies to 16 MiB and applies read/write
timeouts; the API itself is unauthenticated — bind it to localhost.

Startup logs the readiness lines: `ADR store loaded: N invariants`,
`Plan linter ready: N policies`, `Skill governance linter ready`,
`Service catalog loaded: N services`, then
`Platform Planning Gateway listening on <addr>`.

The service catalog is an optional capability with three configurations:

- neither `-services` nor `-service-policy`: catalog disabled — a notice is
  logged and `/discover_service` returns `503 SERVICE_CATALOG_UNAVAILABLE`;
- `-services` alone: the catalog is listable (`GET /services`) but discovery
  stays disabled (logged as a warning);
- both flags: discovery is live. `-service-policy` without `-services`, an
  empty catalog directory, or a ranking policy that fails to compile are all
  startup errors.

## `cmd/svc-mock` — local service stand-in

```bash
svc-mock -addr :9110 -name notify-svc        # POST /v1/messages -> 202 queued
svc-mock -addr :9120 -name payments-gateway  # POST /v1/charges  -> 201 authorized
```

A dependency-free mock of a cataloged service so the discovery tutorial runs
out-of-the-box (the endpoint the catalog returns actually answers). `GET
/healthz` is always `200`; any other `-name` serves a generic echo endpoint.

## `adapters/claudecode/mcpserver` — MCP server (stdio)

Exposes `get_platform_guidelines_for_intent`, `find_platform_service`, and
`lock_in_plan` as native tools; persists the capability ticket through the
per-machine `TokenStore` on a successful lock (see
[capability-ticket.md](capability-ticket.md#storage-layout)).

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--project-dir` | `PPG_PROJECT_DIR` | `os.Getwd()` at spawn | Absolute project directory. The cwd fallback is reliable for Claude Code / Copilot desktop (fresh subprocess per session); set the flag or env explicitly for persistent daemons that survive project switches. |
| `--store-root` | `PPG_STORE_ROOT` | `$XDG_STATE_HOME/ppg` (fallback `~/.local/state/ppg`) | Per-machine state root shared with the guard binaries. |
| — | `PPG_URL` | `http://localhost:8765` | Gateway base URL |

## `adapters/claudecode/guard` — `ppg-guard` PreToolUse hook

Reads the hook JSON from stdin and, for a write tool (Edit, Write, MultiEdit,
NotebookEdit, Update, editFiles, apply_patch, str_replace_editor, create_file,
edit_file, plus any name containing `Edit`/`Write`), verifies the target file —
taken from `file_path`, `path`, or `notebook_path` — against the capability
ticket loaded from the per-machine `TokenStore` (signature, TTL, path scope,
session binding). When the path scope passes it **also** verifies the edited
content: it POSTs the content to the gateway's `/verify_artifact`, and on
`ARTIFACT_REJECTED` blocks with the invariant messages
(`ARCHITECTURAL_INVARIANT_VIOLATION`). It exits 2 with a semantic message on
stderr to block the tool call.

The guard **fails closed**: on any error it cannot recover from (unreadable
payload, unopenable store, unreachable gateway), a PreToolUse edit is blocked
with a message prefixed `PPG_GUARD_ERROR: … (fail-closed)`. `SessionStart` never
blocks.

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--project-dir` | `PPG_PROJECT_DIR` | payload cwd → `os.Getwd()` | Absolute project directory. Falls back to the hook payload's `cwd`, then the process cwd. |
| `--store-root` | `PPG_STORE_ROOT` | `$XDG_STATE_HOME/ppg` | Per-machine state root shared with the MCP server. |
| — | `PPG_URL` | `http://localhost:8765` | Gateway base URL, used to reach `/verify_artifact` for the content check. |

## `adapters/copilot/guard` — `ppg-copilot-guard` PreToolUse hook

Same behavior as `ppg-guard` — including the `/verify_artifact` content check
and the fail-closed stance — but emits Copilot's JSON permission decision
(`permissionDecision: "deny"` with `permissionDecisionReason`) on stdout instead
of exit code 2. Same flags and environment variables (including `PPG_URL`). The
two guards are identical in coverage: same write-tool set, same path fields,
same content policy.

## `cmd/ppg-verify` — apply-time / CI backstop

```bash
ppg-verify [flags]
```

The enforcement leg for surfaces with no in-loop hook (the `gh copilot` CLI,
Cursor, a human at the terminal, CI). It reads the active capability ticket from
the per-machine store, computes the git changeset (`git status --porcelain`,
reading each changed file's current content; deletions are skipped), and POSTs
it to the gateway's `/verify_changeset` — evaluating the changeset-view policy
over the **actual** diff. Wire it as a pre-commit / pre-push hook or a CI step.

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--staged` | — | off (all working-tree changes vs HEAD) | Verify only the staged changes |
| `--plan` | — | — | Path to the locked plan JSON; its hash is also checked against the ticket (`PLAN_SUBSTITUTION` on mismatch) |
| `--project-dir` | `PPG_PROJECT_DIR` | `os.Getwd()` | Absolute project directory |
| `--store-root` | `PPG_STORE_ROOT` | `$XDG_STATE_HOME/ppg` | Per-machine state root shared with the guards |
| `--gateway` | `PPG_URL` | `http://localhost:8765` | Gateway base URL |

Exit codes: `0` = changeset accepted; `1` = rejected (violations printed to
stderr); `2` = could not run the check (no ticket, gateway unreachable) —
fail-closed. See the how-to [Gate changes at apply time](../how-to/gate-changes-at-apply-time.md).

## `adapters/preflight` — black-box pre-flight

```bash
ppg-preflight [-repo <name>] [-stack Go,SQL] "<intent>"
```

| Flag / env | Default | Description |
|---|---|---|
| `-repo` | `checkout-service` | Sent as `repository_context.name` |
| `-stack` | `Go` | Comma-separated, sent as `repository_context.tech_stack` |
| `PPG_URL` | `http://localhost:8765` | Gateway base URL (same convention as the MCP server) |

Writes the enriched invariants to `.cursorrules` and
`.github/copilot-instructions.md` in the current directory.

## Dependencies

| Module | Role |
|---|---|
| `github.com/golang-jwt/jwt/v5` | Capability ticket signing/verification |
| `github.com/open-policy-agent/opa` | Embedded policy engine (Go library — no external OPA binary required) |
| `github.com/modelcontextprotocol/go-sdk` | MCP server for the Claude Code adapter |
| `gopkg.in/yaml.v3` | ADR and skill front-matter parsing |

Why OPA/Rego rather than plain Go policies: see
[dual-representation-adr.md](../explanation/dual-representation-adr.md#why-oparego-for-plan-enforcement).
