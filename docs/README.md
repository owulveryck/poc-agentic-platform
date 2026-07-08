# Documentation

This documentation follows the [Divio](https://docs.divio.com/documentation-system/)
/ [Diátaxis](https://diataxis.fr/) system: four quadrants, four registers,
one directory each.

| You want to… | Quadrant | Start with |
|---|---|---|
| learn by doing | [tutorials/](tutorials/) | [Your first amplified planning cycle](tutorials/01-first-planning-cycle.md) |
| solve a precise task | [how-to/](how-to/) | one recipe per problem, see below |
| check a fact (endpoint, schema, flag, claim) | [reference/](reference/) | [HTTP API](reference/http-api.md) |
| understand *why* it is designed this way | [explanation/](explanation/) | [From vibe coding to governed loops](explanation/from-vibe-coding-to-governed-loops.md) |

## Tutorials — learning-oriented

1. [Your first amplified planning cycle](tutorials/01-first-planning-cycle.md) — the full `enrich → lock_in_plan → smart tool → debt_report` cycle with `curl`
2. [Govern a live Claude Code session](tutorials/02-claude-code-end-to-end.md) — MCP planning + `PreToolUse` gating, end to end
3. [Steer GitHub Copilot with the pre-flight adapter](tutorials/03-github-copilot-preflight.md) — invariants injected into a black-box agent
4. [Validate your first skill](tutorials/04-validate-your-first-skill.md) — the skill governance gate and the security tiers

## How-to guides — task-oriented

- [Add an ADR invariant](how-to/add-an-adr-invariant.md)
- [Write a Rego plan policy](how-to/write-a-rego-plan-policy.md)
- [Retire compensatory scaffolding](how-to/retire-compensatory-scaffolding.md)
- [Add a Smart Tool](how-to/add-a-smart-tool.md)
- [Connect a black-box agent (Copilot / Cursor)](how-to/connect-a-black-box-agent.md)
- [Connect Claude Code](how-to/connect-claude-code.md)
- [Add a skill governance rule](how-to/add-a-skill-governance-rule.md)
- [Gate skill publication in CI](how-to/gate-skill-publication-in-ci.md)

## Reference — information-oriented

- [HTTP API](reference/http-api.md) · [Plan contract](reference/plan-contract.md) · [ADR front matter](reference/adr-front-matter.md)
- [Capability ticket](reference/capability-ticket.md) · [Policy catalog](reference/policy-catalog.md) · [Skill governance](reference/skill-governance.md)
- [Statuses and error codes](reference/error-codes.md) · [Gateway and adapter binaries](reference/gateway-cli.md)

## Explanation — understanding-oriented

- [From vibe coding to governed loops](explanation/from-vibe-coding-to-governed-loops.md)
- [Enrichment and planning](explanation/enrichment-and-planning.md)
- [The dual-representation ADR](explanation/dual-representation-adr.md)
- [Capability tickets and in-tool guards](explanation/capability-tickets-and-in-tool-guards.md)
- [Transition debt](explanation/transition-debt.md)
- [Capability-plane governance (skills)](explanation/capability-plane-governance.md)
- [Design decisions and known limits](explanation/design-decisions-and-limits.md)

Sequence diagrams live in [diagrams/](diagrams/).
