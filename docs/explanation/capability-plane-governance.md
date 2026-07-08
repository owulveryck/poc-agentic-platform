# Capability-plane governance (skills)

The plan linter governs the **execution plane**: what one agent does in one
session. Skills — `SKILL.md` workflows distributed across the organization —
are the **capability plane**: what every agent *can* do, everywhere, at
once. A governed plan built on an ungoverned skill is still a risk: the plan
was checked, but the instructions it followed were not. `POST /validate_skill`
extends the gateway to that plane.

## What a skill is, and why tool mentions drive the tier

A skill body is a **literal instruction set**: the tools it names are the
tools the agent calls when the skill is invoked. That is why the security
tier derives from tool mentions: a body that says "use `Bash`" *is* a skill
that runs shell commands. Tier 0 (read-only) can be auto-approved; tier 1
(file modifications) requires the CI gate and a companion Rego; tier 2
(shell) warrants human review. The PoC derivation is a deliberately naive
substring match — the production posture is a deny-by-default tool
allowlist, which also closes the paraphrase-evasion hole ("run the tests
from the shell" mentions no tool name and lands in tier 0).

## Repository versus registry

Any git host can be a skill *repository* — decentralized authoring is a
feature. The enterprise *registry* is different: it is the **trust
boundary**. Publication into the registry is where provenance is recorded
and governance is enforced; installation out of it is where trust is
consumed. The npm analogy holds: anyone can push code to GitHub, but what
your build pulls comes from a registry with a policy gate.

## The dual-representation skill

The dual-representation pattern of the
[ADRs](dual-representation-adr.md) applies verbatim:

| Representation | Consumed by | Moment |
|---|---|---|
| `SKILL.md` body (semantic directive) | The agent | At invocation |
| Companion `SKILL.rego` | Skill Linter; Plan Linter | At publish; at `lock_in_plan` |

The companion Rego lets a skill *export* governance requirements: when a
plan declares it executes a skill, the plan linter can evaluate the union of
the ADR policies and the skill's companion policies.

## The three gates

| Gate | Moment | Mechanism | In this PoC |
|---|---|---|---|
| 1 — Publish | Skill enters the registry | CI calls `POST /validate_skill` | ✅ implemented |
| 2 — Install | Skill enters a workstation/project | Revalidation against current (possibly tightened) policies; content hashes detect tampering | ❌ not implemented |
| 3 — Runtime | A plan carries a `skill_id` | Plan linter evaluates the skill's companion Rego alongside the ADR policies | ❌ not implemented |

The PoC implements Gate 1 only; Gates 2 and 3 are the production path
(see `AUDIT.md` for the full gap analysis, including the version-skew window
between publish and runtime that content-hash pinning closes).

## Debt applied to skills

Skills sit on the same durability axis as every other artifact: an
amplifier skill encodes a durable organizational workflow; a compensatory
skill works around a current model limitation and should carry a sunset
condition in its companion Rego. Enforcing that (and folding skills into
`GET /debt_report`) is the next natural extension — see
[transition-debt.md](transition-debt.md).

> 📖 Conceptual reference: the blog article *"The Governed Skills Registry:
> Policy-as-Code for Enterprise Agent Capabilities"*.
