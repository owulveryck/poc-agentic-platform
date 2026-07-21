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
| look up a term | [explanation/glossary.md](explanation/glossary.md) | canonical vocabulary per [ADR-130](decisions/ADR-130-gateway-naming.md) |

## Golden path — from zero to a governed skill

Two wiring models exist and are **alternatives**: per-project (tutorial 2
registers ppg inside one project — good for a first experiment) and
**governed workstation** (the reference mode: install once, every project
on the machine is governed). Don't mix them on the same machine; the
sequence below is the workstation path:

1. [Tutorial 0](tutorials/00-bootstrap.md) — build, install, start the
   validation server (`make install`, `scripts/setup-gateway-service.sh`).
2. [Set up a governed workstation](how-to/set-up-a-governed-workstation.md)
   — hooks + MCP at user scope (or managed scope for fleets).
3. [Tutorial 10](tutorials/10-claude-on-governed-workstation.md) — watch
   a fresh, unconfigured project get governed.
4. [Bundle validation with a skill](how-to/bundle-validation-with-a-skill.md)
   — author your own SKILL.md + SKILL.rego package.
5. [Tutorial 15](tutorials/15-skill-only-enforcement.md) — see the skill
   enforce itself with zero ADRs, under an adversarial prompt.

## Tutorials — learning-oriented

0. [Bootstrap the platform on your machine](tutorials/00-bootstrap.md) — one-time install of the validation server, adapter binaries, and MCP server registration. Every other tutorial's prereqs collapse to this.
1. [Your first amplified planning cycle](tutorials/01-first-planning-cycle.md) — the full `enrich → lock_in_plan → smart tool → debt_report` cycle with `curl`
2. [Govern a live Claude Code session](tutorials/02-claude-code-end-to-end.md) — MCP planning + `PreToolUse` gating, end to end
3. [Steer GitHub Copilot with the pre-flight adapter](tutorials/03-github-copilot-preflight.md) — invariants injected into a black-box agent
4. [Validate your first skill](tutorials/04-validate-your-first-skill.md) — the skill governance gate and the security tiers
5. [Write your first ADR, end to end](tutorials/05-write-your-first-adr.md) — author an invariant (Markdown + Rego) and watch both halves act
6. [From a governed skill to a governed session](tutorials/06-skill-to-session-end-to-end.md) — write a skill and its policy, pass the publication gate, watch it drive a session through every control point
7. [Govern a live GitHub Copilot session](tutorials/07-copilot-end-to-end.md) — the Copilot sibling of tutorial 2: MCP planning + `PreToolUse` gating in the official Copilot app
8. [Govern a design system through a skill](tutorials/08-design-system-end-to-end.md) — extend the loop from path-scope to content-scope: enforce the Deep Umbra palette on buttons via ADR-090's Rego at the artifact altitude (no per-skill hook)
9. [Copilot on a governed workstation](tutorials/09-copilot-on-governed-workstation.md) — with the workstation configured user-wide (see how-to below), a fresh project is governed with three commands and one prompt
10. [Claude Code on a governed workstation](tutorials/10-claude-on-governed-workstation.md) — the same demo for Claude Code
11. [With and without the validation server — a side-by-side demo](tutorials/11-with-and-without-the-gateway.md) — same skill, same prompts, two projects: without the platform a small model drifts under an adversarial prompt; with the platform, the drift is deterministically refused
12. [Try to bypass the validation server (and watch it hold)](tutorials/12-bypassing-the-gateway.md) — a red-team catalogue of every trick against the Claude Code loop (no ticket, out-of-scope, traversal, sibling-prefix, forbidden content, session replay, tampered/forged ticket, server-down, self-disable) paired with its refusal — plus the honest limits caught only at apply time by `ppg-verify`; asserted end-to-end by `scripts/redteam-bypass.sh`
13. [Discover and use a platform service](tutorials/13-discover-a-platform-service.md) — the service catalog: the agent asks the validation server which sanctioned service provides a capability (notifications, payments), gets the endpoint + API usage ranked by policy, and is refused when it reaches for a deprecated/forbidden provider; asserted by `scripts/service-catalog-demo.sh`
14. [With and without the validation server, on Claude Code](tutorials/14-with-and-without-claude-code.md) — the Claude Code companion of tutorial 11: same design-system skill, same small-model drift, same deterministic refusals, using the repo's setup/teardown scripts to toggle the platform on and off
15. [Skill-only enforcement, on Claude Code](tutorials/15-skill-only-enforcement.md) — the same four-Act demo as tutorial 14, but the enforcement comes exclusively from `demo/skills/design-system/SKILL.rego`: no ADR-090, no ADR-120 in scope. One skill, its companion Rego, three views — the shape a design team ships as a stand-alone bundle

## How-to guides — task-oriented

- [Add an ADR invariant](how-to/add-an-adr-invariant.md)
- [Rego survival kit](how-to/rego-survival-kit.md) (no prior Rego knowledge required)
- [Write a Rego plan policy](how-to/write-a-rego-plan-policy.md)
- [Retire compensatory scaffolding](how-to/retire-compensatory-scaffolding.md)
- [Add a Smart Tool](how-to/add-a-smart-tool.md)
- [Enforce a content invariant](how-to/enforce-a-content-invariant.md) — the pattern behind tutorial 8, generalized: an ADR's artifact-view Rego rule, enforced by the guard and `ppg-verify`
- [Gate changes at apply time](how-to/gate-changes-at-apply-time.md) — `ppg-verify` as a pre-commit / pre-push / CI backstop for hookless surfaces
- [Connect a black-box agent (Copilot / Cursor)](how-to/connect-a-black-box-agent.md)
- [Connect Claude Code](how-to/connect-claude-code.md)
- [Set up a governed workstation](how-to/set-up-a-governed-workstation.md) — install the platform user-wide so every project on this machine is governed by default; tutorials 9 and 10 demonstrate the result
- [Bundle validation with a skill](how-to/bundle-validation-with-a-skill.md) — author a SKILL.md + SKILL.rego package whose enforcement travels with it (the tutorial-15 shape)
- [Add a skill governance rule](how-to/add-a-skill-governance-rule.md) — extend the *registry publish gate* (`skill-governance/*.rego`), not an individual skill's policy
- [Gate skill publication in CI](how-to/gate-skill-publication-in-ci.md)
- [Resolve a policy conflict](how-to/resolve-a-policy-conflict.md) — the human half of `POLICY_CONFLICT`: inspect the escalation with `ppg escalations`, fix the corpus, close the conflict so it cannot recur
- [Add a service to the catalog](how-to/add-a-service-to-the-catalog.md) — make a shared capability discoverable and, when needed, enforced

## Reference — information-oriented

- [HTTP API](reference/http-api.md) · [Plan contract](reference/plan-contract.md) · [ADR front matter](reference/adr-front-matter.md)
- [Capability ticket](reference/capability-ticket.md) · [Policy catalog](reference/policy-catalog.md) · [Policy views](reference/policy-views.md) · [Skill governance](reference/skill-governance.md)
- [Service catalog](reference/service-catalog.md) — record schema, ranking policy, `/discover_service` + `/services` endpoints
- [Statuses and error codes](reference/error-codes.md) · [Validation server and control-point binaries](reference/validation-server-cli.md)

## Explanation — understanding-oriented

- [From vibe coding to governed loops](explanation/from-vibe-coding-to-governed-loops.md)
- [Enrichment and planning](explanation/enrichment-and-planning.md)
- [The dual-representation ADR](explanation/dual-representation-adr.md)
- [Capability tickets and in-tool guards](explanation/capability-tickets-and-in-tool-guards.md)
- [Transition debt](explanation/transition-debt.md)
- [Capability-plane governance (skills)](explanation/capability-plane-governance.md)
- [Design decisions and known limits](explanation/design-decisions-and-limits.md)

Sequence diagrams live in [diagrams/](diagrams/).
