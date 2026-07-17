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
- Skill governance covers Gate 1 only (publish): no install-time
  revalidation, no runtime enforcement of companion policies. The security
  tier and the verb/secret checks are substring and pattern matches, open
  to paraphrase evasion — see
  [capability-plane-governance.md](capability-plane-governance.md).
- No persistence or telemetry yet: that is the third pillar (*observation*),
  intentionally out of scope here.
