---
adr_id: ADR-211
title: A BMAD dev plan is planned against its story (the story is read before code)
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["bmad", "story", "dev-story", "implement", "epic", "sprint", "feature"]
enforcement:
  mode: programmatic
  policy_id: bmad_plan_references_story
  rego: ADR-211.rego
  altitudes: [plan]
---

## Invariant

In a BMAD implementation cycle, code follows the story. Any plan that writes
implementation code (a step targeting a file under `src/`) MUST also include a
step that **reads the story** it implements (a step whose targets include the
story file — `.../implementation-artifacts/…`, `.../stories/…`, or `*.story.md`).

The plan cannot lock until the story is acknowledged as an input. This is the
BMAD analogue of ADR-203 (API changes are contract-first: the plan must read the
OpenAPI spec). The story is the Dev agent's contract.

## Rationale (durability)

The whole point of BMAD's "context-engineered" stories is that the Dev agent
implements *from the story*, not from a vague memory of the epic. A plan that
jumps straight to `src/` with no story-reading step is the silent-drift failure
mode: the agent implements what it *thinks* the story said.

This is an **amplifier**, `sunset_condition: null`: a smarter model reads the
story and reasons over it more thoroughly; it never makes acknowledging the
contract useless. It is a coordination invariant, not a crutch for a weak model.

## What we do NOT write here

We do not check that the code *satisfies* the acceptance criteria — that is deep
semantic conformance (judgment / a downstream review concern, Foyer B). We force
the story to be *read as part of the plan*, exactly as ADR-203 forces the
contract to be read, not its deep satisfaction.

## Enforcement altitude

Plan altitude only (`input.view == "plan"`), mirroring `ADR-203.rego`. Evaluated
at `/lock_in_plan`: a plan targeting `src/` with no story-reading step is
rejected before any code is written. Because it reasons about the *set of steps*
(is a read-story step present?), it cannot be re-expressed at a content altitude —
a single file's bytes carry no evidence that a story was read (same limitation as
`demo/skills/add-payment-method/SKILL.rego`).
