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

0. [Bootstrap the platform on your machine](tutorials/00-bootstrap.md) — one-time install of the gateway, adapter binaries, and MCP server registration. Every other tutorial's prereqs collapse to this.
1. [Your first amplified planning cycle](tutorials/01-first-planning-cycle.md) — the full `enrich → lock_in_plan → smart tool → debt_report` cycle with `curl`
2. [Govern a live Claude Code session](tutorials/02-claude-code-end-to-end.md) — MCP planning + `PreToolUse` gating, end to end
3. [Steer GitHub Copilot with the pre-flight adapter](tutorials/03-github-copilot-preflight.md) — invariants injected into a black-box agent
4. [Validate your first skill](tutorials/04-validate-your-first-skill.md) — the skill governance gate and the security tiers
5. [Write your first ADR, end to end](tutorials/05-write-your-first-adr.md) — author an invariant (Markdown + Rego) and watch both halves act
6. [From a governed skill to a governed session](tutorials/06-skill-to-session-end-to-end.md) — write a skill and its policy, pass the publication gate, watch it drive a session through every gateway
7. [Govern a live GitHub Copilot session](tutorials/07-copilot-end-to-end.md) — the Copilot sibling of tutorial 2: MCP planning + `PreToolUse` gating in the official Copilot app
8. [Govern a design system through a skill](tutorials/08-design-system-end-to-end.md) — extend the loop from path-scope to content-scope: enforce the Deep Umbra palette on buttons with a shell-script PreToolUse hook shipped inside the skill

## How-to guides — task-oriented

- [Add an ADR invariant](how-to/add-an-adr-invariant.md)
- [Rego survival kit](how-to/rego-survival-kit.md) (no prior Rego knowledge required)
- [Write a Rego plan policy](how-to/write-a-rego-plan-policy.md)
- [Retire compensatory scaffolding](how-to/retire-compensatory-scaffolding.md)
- [Add a Smart Tool](how-to/add-a-smart-tool.md)
- [Enforce a content invariant with a PreToolUse hook](how-to/enforce-a-content-invariant.md) — the pattern behind tutorial 8, generalized for any content-scope policy
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
