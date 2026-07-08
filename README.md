# poc-agentic-platform — Platform Planning Gateway (PoC)

A proof of concept for the **amplified agentic loop**: instead of front-loading
all the governance in the initial prompt and blocking the agent after the
fact, the platform injects governance *inside* each step of the loop:
planning, execution, observation.

Companion repository of the blog articles
[The Amplified Agentic Loop: Guardrails as Accelerators](https://blog.owulveryck.info/2026/07/07/amplified-agentic-loop.html)
and *The Governed Skills Registry: Policy-as-Code for Enterprise Agent
Capabilities*.

> **Status**: proof of concept, not production-ready by design
> (symmetric hard-coded JWT secret, keyword-based ADR retrieval, simulated
> staging state). `AUDIT.md` tracks the conformance of the code against the
> articles.

## What it demonstrates

| Component | Role | Durability nature |
|---|---|---|
| `POST /enrich` | Retrieves **architectural invariants** from the ADR store for an intent (no hard-coded business pattern) | amplifier / declarative |
| `POST /lock_in_plan` | **Deterministic plan linter** (OPA/Rego, not an LLM): rejects with semantic violations, or issues a **capability ticket** (ephemeral JWT: plan hash + least-privilege scope) | amplifier / programmatic |
| `POST /tools/{name}` | **Smart Platform Tools**: verify the ticket in-tool (`OUT_OF_PLAN_SCOPE` refusal), execute in a sandbox, return **semantic feedback** (`remediation_guidance`) | amplifier (+ one tagged compensatory translator) |
| `GET /debt_report` | **Transition-debt governance**: every artifact is tagged `amplifier` or `compensatory`; compensatory ones carry a measurable sunset condition; the ratio must trend to 0 | governance |
| `POST /validate_skill` | **Skill governance linter** (OPA/Rego): publish gate for enterprise skills, structural rules + security tier (0/1/2) | amplifier / programmatic |

## Quick start

```bash
go run ./cmd/ppg          # listens on :8000 (use -addr :8765 if busy)
```

Then follow the [first tutorial](docs/tutorials/01-first-planning-cycle.md)
(full cycle with `curl`), wire a **stock Claude Code session** to the gateway
([tutorial 2](docs/tutorials/02-claude-code-end-to-end.md)), steer **GitHub
Copilot** through the pre-flight adapter
([tutorial 3](docs/tutorials/03-github-copilot-preflight.md)), or validate a
skill against the governance gate
([tutorial 4](docs/tutorials/04-validate-your-first-skill.md)).

## Documentation (Divio / Diátaxis system)

Index: [docs/README.md](docs/README.md)

| You want to… | Read |
|---|---|
| learn by running the platform | [docs/tutorials/](docs/tutorials/) |
| solve a precise task (add an ADR, a Rego policy, a governance rule, retire scaffolding) | [docs/how-to/](docs/how-to/) |
| check an endpoint, a schema, a JWT claim, a flag | [docs/reference/](docs/reference/) |
| understand *why* it is designed this way | [docs/explanation/](docs/explanation/) |

## Layout

```
cmd/ppg/                 HTTP gateway (enrich, lock_in_plan, tools, debt_report, validate_skill)
internal/adr/            ADR store loading + invariant retrieval
internal/enrich/         amplifier context builder
internal/plan/           structured plan contract (see schemas/plan.schema.json)
internal/linter/         OPA/Rego plan linter, policies tagged amplifier|compensatory
internal/ticket/         capability ticket (JWT: plan_hash + scope, 15 min TTL)
internal/smarttools/     ticket guard + sandbox + semantic analyzers
internal/skill/          skill parsing + OPA/Rego governance linter + security tiers
internal/debt/           transition-debt report
adr/                     the ADR corpus (YAML front matter + invariant text + paired .rego)
skill-governance/        skill governance policies (structure.rego, security.rego)
schemas/                 language-neutral JSON Schema of the plan contract
adapters/preflight/      black-box adapter (writes .cursorrules / copilot-instructions.md)
adapters/claudecode/     Claude Code adapter: MCP server (planning) + PreToolUse hook (gating)
docs/                    Diátaxis documentation + PlantUML diagrams
```
