# poc-agentic-platform — Platform Planning Gateway (PoC)

A proof of concept for the **amplified agentic loop**: instead of front-loading
all the governance in the initial prompt and blocking the agent after the
fact, the platform injects governance *inside* each step of the loop:
planning, execution, observation.

Companion repository of the blog article
[The Amplified Agentic Loop: Guardrails as Accelerators](https://blog.owulveryck.info/2026/07/07/amplified-agentic-loop.html).

> **Status**: proof of concept, not production-ready by design
> (symmetric hard-coded JWT secret, keyword-based ADR retrieval, simulated
> staging state).

## What it demonstrates

| Component | Role | Durability nature |
|---|---|---|
| `POST /enrich` | Retrieves **architectural invariants** from the ADR store for an intent (no hard-coded business pattern) | amplifier / declarative |
| `POST /lock_in_plan` | **Deterministic plan linter** (not an LLM): rejects with semantic violations, or issues a **capability ticket** (ephemeral JWT: plan hash + least-privilege scope) | amplifier / programmatic |
| `POST /tools/{name}` | **Smart Platform Tools**: verify the ticket in-tool (`OUT_OF_PLAN_SCOPE` refusal), execute in a sandbox, return **semantic feedback** (`remediation_guidance`) | amplifier (+ one tagged compensatory translator) |
| `GET /debt_report` | **Transition-debt governance**: every artifact is tagged `amplifier` or `compensatory`; compensatory ones carry a measurable sunset condition; the ratio must trend to 0 | governance |

## Quick start

```bash
go run ./cmd/ppg          # listens on :8000 (use -addr :8765 if busy)
```

Then follow the [tutorial](docs/tutorial.md) (full cycle with `curl`), or wire
a **stock Claude Code session** to the gateway (MCP tools for planning, a
`PreToolUse` hook for deterministic in-tool gating) with
[adapters/claudecode](adapters/claudecode/README.md).

## Documentation (Divio system)

| You want to… | Read |
|---|---|
| learn by running the full cycle | [docs/tutorial.md](docs/tutorial.md) |
| solve a precise task (add an ADR, a policy, retire scaffolding) | [docs/how-to.md](docs/how-to.md) |
| check an endpoint, a schema, a JWT claim | [docs/reference.md](docs/reference.md) |
| understand *why* it is designed this way | [docs/explanation.md](docs/explanation.md) |

## Layout

```
cmd/ppg/                 HTTP gateway (enrich, lock_in_plan, tools, debt_report)
internal/adr/            ADR store loading + invariant retrieval
internal/enrich/         amplifier context builder
internal/plan/           structured plan contract (see schemas/plan.schema.json)
internal/linter/         deterministic policies, tagged amplifier|compensatory
internal/ticket/         capability ticket (JWT: plan_hash + scope, 15 min TTL)
internal/smarttools/     ticket guard + sandbox + semantic analyzers
internal/debt/           transition-debt report
adr/                     the ADR corpus (YAML front matter + invariant text)
adapters/preflight/      black-box adapter (writes .cursorrules / copilot-instructions.md)
adapters/claudecode/     Claude Code adapter: MCP server (planning) + PreToolUse hook (gating)
docs/                    Divio documentation + PlantUML diagrams
```
