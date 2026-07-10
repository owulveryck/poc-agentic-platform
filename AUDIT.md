# Audit — code vs. reference articles

Date: 2026-07-08. Scope: conformance of this repository against the two
articles it accompanies:

- **A1** — *The Amplified Agentic Loop: Guardrails as Accelerators*
  (blog.owulveryck.info, 2026-07-07, published)
- **A2** — *The Governed Skills Registry: Policy-as-Code for Enterprise
  Agent Capabilities* (2026-07-08, draft at audit time)

Every "verified live" entry below was exercised against a running gateway
(`go run ./cmd/ppg -addr :8765`) on the audit date.

Statuses: ✅ conforms · 🟡 partial · ❌ not implemented · 📄 article-only
(described as production path, no code claim).

## A1 — Amplified Agentic Loop

| Claim (article) | Code location | Status |
|---|---|---|
| `enrich()` soft move: ADR retrieval via scope selectors, no hard-coded business pattern | `internal/adr`, `internal/enrich`, `POST /enrich` | ✅ verified live (ADR-042 + ADR-070 returned for a payment intent) |
| `lock_in_plan()` hard move: OPA/Rego linter, deterministic 422 with semantic violations | `internal/linter`, `adr/*.rego` (package `ppg.linter`) | ✅ verified live (`go_tests_present` rejection, then `PLAN_LOCKED`) |
| Capability ticket: ephemeral signed JWT, plan fingerprint + least-privilege scope | `internal/ticket` (HS256, TTL 15 min, `plan_hash`, `scope`) | ✅ verified live (claims decoded and matched the locked plan) |
| Smart Tools: in-tool ticket check, sandbox, semantic errors with `remediation_guidance` | `internal/smarttools/{patchcode,dbmigrate,translate}` | ✅ verified live (`OUT_OF_PLAN_SCOPE`, `GO_SYNTAX_ERROR`, `DATABASE_SCHEMA_CONFLICT`) |
| Dual-representation ADRs; ADR-042 intentionally declarative-only | `adr/` (4 ADRs, 3 paired `.rego`) | ✅ |
| Debt report: tagged artifacts, sunset conditions, PoC ships in `DEBT_ALERT` (2/5) | `internal/debt`, `GET /debt_report` | ✅ verified live (`transition_debt_ratio: 0.4`, 2 pending sunsets) |
| Claude Code adapter: stdio MCP server, 2 tools, ticket in `.ppg-ticket` | `adapters/claudecode/mcpserver` | ✅ |
| `ppg-guard` PreToolUse hook on `Edit\|Write`, exit 2, semantic stderr | `adapters/claudecode/guard` | ✅ verified live (block out-of-scope, pass in-scope, block without ticket) |
| Copilot path: pre-flight writes `.github/copilot-instructions.md` | `adapters/preflight` | ✅ verified live; gateway URL/repo-context hardcoding fixed during this audit (see below) |
| MCP tool schema auto-generated from `internal/plan#Plan` | `modelcontextprotocol/go-sdk` typed tools | ✅ |
| Docs follow Divio/Diátaxis; repo doubles as a documentation template | `docs/` | ✅ after this audit — was 🟡 (4 monolithic files with stale sections; see "Documentation" below) |
| Pillar 3 (retroactive observation) out of scope | — | 📄 consistent (no code, none claimed) |

## A2 — Governed Skills Registry

| Claim (article) | Code location | Status |
|---|---|---|
| `POST /validate_skill`: `SKILL_VALID` + tier, or `SKILL_REJECTED` + violations with `nature` | `cmd/ppg`, `internal/skill` | ✅ verified live (4 violations on a bad skill; tier 0/1/2 probes) |
| Governance Rego in `skill-governance/` (`structure.rego`, `security.rego`, package `ppg.skills.governance`) | `skill-governance/` | ✅ |
| Structural rules: name kebab-case ≤32, description 50–500 chars, semver required, argument-hint with `$ARGUMENTS` | `structure.rego` | ✅ |
| Structural rule: description **starts with a verb** | `structure.rego` | ✅ implemented 2026-07-10 (naive `^[A-Z][a-z]+s\s` pattern, assumed) |
| Structural rule: body **≤ 500 lines** | `structure.rego` | ✅ implemented 2026-07-10 |
| Structural rule: **no hardcoded secrets** | `structure.rego` | ✅ implemented 2026-07-10 (pattern scan: AWS keys, PEM, inline assignments) |
| Companion Rego required for tier ≥ 1 | `security.rego` (`privileged`) | ✅ fixed 2026-07-10: fires on `Edit`/`Write`/`Bash`; a Bash-only skill no longer escapes |
| Security tiers 0/1/2 from tool mentions, deliberately keyword-based | `skill.Linter.Tier` (Go substring match) | ✅ as described; the article itself flags paraphrase evasion (production: deny-by-default allowlist). Note: tier logic exists in Go while `modifies_files` re-implements it in Rego — two sources of truth |
| Gate 1 (publish, CI) | `/validate_skill` | ✅ (recipe: `docs/how-to/gate-skill-publication-in-ci.md`) |
| Gate 2 (install revalidation, content hashes) | — | ❌ described as registry-side production path |
| Gate 3 (plan carries `skill_id`; plan linter unions companion Rego) | — | ❌ `plan.Plan` has no `skill_id` field; `linter.New` loads ADR regos only |
| Compensatory skills carry `sunset_condition`; skills folded into debt report | — | ❌ article says "next natural extension" — consistent, but unimplemented |
| `versioning`: version-skew window closed by hash pinning | — | 📄 production path |

## Bugs found and fixed during this audit

| Issue | Location | Fix |
|---|---|---|
| **Fail-open linters**: an OPA result that failed to marshal/unmarshal made `Validate` return `nil` — a malformed policy output silently **locked the plan / published the skill** | `internal/linter/linter.go`, `internal/skill/linter.go` | Fail closed: synthetic `linter_eval_error` violation; regression tests with a deliberately malformed policy (`testdata/BAD-001.rego`, `testdata/badshape/`) |
| Panic on empty `targets` (`targets[0]`), reachable from the raw `/tools/{name}` request body | `internal/smarttools/patchcode/patchcode.go` | Guard + `EXECUTION_FAILED`; test |
| Panic on truncated `CREATE TABLE ` statement (`fields[2]`) | `internal/smarttools/dbmigrate/dbmigrate.go` | Length guard + `EXECUTION_FAILED`; test |
| Pre-flight hardcoded `http://localhost:8000` (no `PPG_URL`, unlike the MCP server) and hardcoded repo context `checkout-service`/`["Go"]` | `adapters/preflight/main.go` | `PPG_URL` env + `-repo`/`-stack` flags; tests |
| Ticket was a pure bearer capability: a new session within the 15-min TTL inherited `.ppg-ticket`, and the `session_id` claim was agent-chosen and never checked (post-audit finding, 2026-07-10) | `adapters/claudecode/{guard,mcpserver}` | Session binding: `SessionStart` hook records the real session id (`.ppg-session`) and purges leftover tickets; the MCP server stamps it into the plan at lock; the guard blocks `SESSION_MISMATCH`; tests |

## Known limits kept as-is (assumed PoC posture, documented)

- Symmetric hard-coded JWT secret (`internal/ticket`); production: KMS,
  asymmetric keys.
- Keyword-based ADR retrieval; production: embeddings + reranking.
- Simulated sandbox and staging state (`patchcode`, `dbmigrate`).
- Tier derivation by substring match (see A2 table).
- Duplications accepted at PoC scale: front-matter parsing
  (`internal/adr` vs `internal/skill`), OPA eval boilerplate + `Violation`
  structs (`internal/linter` vs `internal/skill`), testdata Rego copies of
  production Rego (drift risk — the skill testdata already lacks the
  \>500-char description rule).
- `enrich.Enrich` accepts and ignores `RepoContext` (reserved for
  stack-aware retrieval).
- `smarttools.ToolMeta.Nature/SunsetCondition` are registered but never
  consumed by the debt report (the generic translator is hardcoded in
  `internal/debt`).
- `schemas/plan.schema.json` declares `session_id` as `format: uuid`; the Go
  structural validation only checks non-emptiness.

## Documentation

Before this audit: 4 monolithic Divio files, correct in register but stale
since the OPA integration — dependencies section omitted OPA and the MCP
SDK, a broken anchor (`explanation.md#why-plain-go-policies`), the
"add a policy" recipe described the pre-OPA Go registration, the ADR
front-matter table omitted `enforcement.rego`, the tutorial said Go 1.23+
(go.mod: 1.25), and the entire skill-governance feature
(`/validate_skill`, `skill-governance/`, `-skill-governance`) was
undocumented.

After: split into `docs/{tutorials,how-to,reference,explanation}/` with an
index (`docs/README.md`), one file per topic, all staleness fixed, skill
governance covered in all four quadrants, and two end-to-end agent
tutorials (Claude Code, GitHub Copilot) whose commands were executed against
a live gateway. `docs/tutorial.md` and `docs/explanation.md` remain as
redirect stubs because the published article links to those paths.
