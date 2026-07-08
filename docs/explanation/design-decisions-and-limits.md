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

## Known limits of the PoC

- Keyword-based invariant retrieval (production: embeddings + reranking).
- Symmetric hard-coded JWT secret (production: KMS, asymmetric keys).
- Simulated sandbox (in-memory parse / staging state): no real workspace.
- Hard gating is not guaranteed on black-box tools (Copilot/Cursor).
- Skill governance covers Gate 1 only (publish): no install-time
  revalidation, no runtime enforcement of companion policies, and the
  security tier is a substring match open to paraphrase evasion — see
  [capability-plane-governance.md](capability-plane-governance.md).
- No persistence or telemetry yet: that is the third pillar (*observation*),
  intentionally out of scope here.
