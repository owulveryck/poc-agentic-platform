# Design decisions and known limits

## Alternatives considered and rejected

| Option | Why rejected |
|---|---|
| Everything in the system prompt | Fragile, non-deterministic, unverifiable: declarative compensatory. |
| Gate only in CI (post-hoc) | Purely after-the-fact: frustrates the flow, feedback arrives too late for self-correction. |
| An LLM to validate the plan | Non-deterministic → does not solve weak-model drift. We want a *linter*, not a judge. |
| Hard-coding business patterns in the gateway | Compensatory debt; does not scale; gains nothing from model progress. |

## Team Topologies positioning

The gateway is a **platform product** (internal SaaS) built by the Platform
team to reduce the cognitive load of stream-aligned teams and their agents.
ADRs are co-owned with the architects; programmatic policies belong to the
platform team; stream teams consume everything friction-free via MCP or the
pre-flight adapter. X-as-a-Service applied to agentic governance.

## Concurrency

The gateway is safe to drive with concurrent requests: every dependency (ADR
store, the OPA prepared query, the linters, the ticket config, the tool
catalog) is built at startup and only read while serving, and each policy
evaluation uses per-call input — so it is effectively stateless per request (a
concurrency test exercises all endpoints under `-race`). The per-machine store
is shared by independent short-lived processes (the guards, the MCP server,
`ppg-verify`); it serializes them with a per-project advisory `flock` plus
atomic-rename writes, so concurrent sessions cannot corrupt or half-purge state
(see [capability-ticket.md](../reference/capability-ticket.md#storage-layout)).

## Known limits of the PoC

- Keyword-based invariant retrieval (production: embeddings + reranking).
- Symmetric JWT key. Since v1.0.0 it is no longer hard-coded — it comes
  from `$PPG_TICKET_SECRET` or a per-machine key file generated on first
  run — but it remains symmetric (production: KMS, asymmetric keys).
- Simulated sandbox (in-memory parse / staging state): no real workspace.
- Hard gating is layered by surface, not absent anywhere. Claude Code
  (`ppg-guard`), Copilot desktop and VS Code Copilot Chat (`ppg-copilot-guard`)
  get the **in-loop** half: every edit is checked for path scope *and* content
  (the artifact view of the Rego corpus, via `/verify_artifact`), and the guards
  fail closed. Hookless surfaces (`gh copilot` CLI, Cursor, a human, CI) get the
  **apply-time** half: `ppg-verify` evaluates the same corpus over the whole diff
  (`/verify_changeset`) as a pre-commit / pre-push / CI step. Content invariants
  are therefore enforced deterministically, not left to prose — the residual gap
  is only the *window* before an apply-time check runs on the hookless surfaces.
- Skill governance covers Gate 1 (publish) and Gate 3 (plan-time
  companion Rego, since v1.0.0). At the content altitudes (artifact +
  changeset) companion policies apply with **union semantics**: every
  registered skill applicable to the session is evaluated against every
  edit, whether or not the plan declared a `skill_id` — only a skill's
  plan-view *workflow requirements* stay declaration-scoped. The security
  tier and the verb/secret checks are substring and pattern matches, open
  to paraphrase evasion — see
  [capability-plane-governance.md](capability-plane-governance.md).
- **No conflict detection between validations.** Two policies that can
  never both pass — e.g. a skill companion *requiring* a plan step that
  an ADR *forbids* — surface as an ordinary `PLAN_REJECTED` carrying the
  union of both violation lists. The linter does not diagnose that the
  rule set itself is unsatisfiable: the agent is told to "fix the
  violations above and resubmit" although no legal plan exists, and only
  a human reading the loop notices. The shipped corpus avoids the one
  known near-conflict (ADR-090 requires a Read of `design/tokens.css`,
  ADR-120 forbids writes to it) by rule design plus a dedicated test —
  i.e. by policy-author discipline, not by mechanism. General
  unsatisfiability checking is undecidable; the implemented mitigation is
  the deterministic *livelock* escalation: 3 consecutive rejections of a
  session with a byte-identical violation set flip the response to a
  hard-blocking `409 POLICY_CONFLICT` naming the clashing policies and
  their sources (adr / skill / built-in), and append a record to
  `$XDG_STATE_HOME/ppg/escalations.jsonl` for the humans who own the
  rules — see [error codes](../reference/error-codes.md). This detects
  the livelock *symptom*, not unsatisfiability in general.
- **Session-scoped skill registration is memory-only and single-tenant.**
  The `POST /register_skill` endpoint stores companion Rego per session in
  a process-local map (`internal/linter/linter.go` — `sessionSkills`); a
  gateway restart drops every uploaded skill. The MCP server self-heals
  the resulting `unknown_skill` on the next `/lock_in_plan` with a
  one-shot retry (`lockWithRegistrationRetry`), but longer-lived
  persistence (Bolt/SQLite/Redis) is not implemented. The endpoint also
  trusts `session_id` from the client — the same posture as the JWT
  ticket key — so isolation between users relies on session ids being
  unguessable UUIDs (`ppg-guard`'s `SessionStart` emits UUIDv4, which is
  sufficient for a trusted-team deployment on a private network). A
  hostile-multi-tenant deployment needs mTLS on `/register_skill` and
  signed skill manifests, in the same "enterprise-grade" bucket as
  asymmetric ticket keys. See the
  [/register_skill reference](../reference/http-api.md#post-register_skill)
  for the concrete lifetime & auth paragraphs.
- No persistence or telemetry yet: that is the third pillar (*observation*),
  intentionally out of scope here.
