---
adr_id: ADR-210
title: A BMAD story carries its mandatory sections (Acceptance Criteria + Tasks)
status: accepted
nature: compensatory
sunset_condition: "Remove once the BMAD create-story workflow is enforced upstream (its own validate-create-story gate run in CI, rejecting incomplete stories before they land). Until then this is a content tripwire against silently truncated stories."
scope_selectors: ["bmad", "story", "epic", "sprint", "backlog", "acceptance", "dev-story"]
enforcement:
  mode: programmatic
  policy_id: bmad_story_schema_complete
  rego: ADR-210.rego
  altitudes: [artifact, changeset]
---

## Invariant

A BMAD *story* file — the hand-off artifact the Scrum-Master agent produces for
the Dev agent — MUST contain its two load-bearing sections:

- `## Acceptance Criteria`
- `## Tasks / Subtasks`

These are exactly the sections the stock BMAD story template
(`bmad-create-story/template.md`) ships with. A story that reaches the Dev agent
without them is not a BMAD story; it is a title and a wish.

A file is treated as a story when it lives where BMAD writes stories
(`.../implementation-artifacts/*.md`, any `.../stories/*.md`) or is named
`*.story.md`.

## Rationale (why compensatory, not amplifier)

BMAD's own instruction already *says* the story must have acceptance criteria.
The failure mode this ADR guards against is not "the method is wrong" but "the
model **claims** it followed the method and silently shipped a story without the
criteria" — the drift that BMAD, being advisory, cannot catch on its own.

This is a **tripwire**: it exists to compensate for a model that under-fills the
template. A stronger model that reliably fills the template makes it redundant —
hence a real `sunset_condition`. Contrast the amplifier ADR-211, which stays
useful no matter how smart the model gets.

## What we do NOT write here

We do not judge the *quality* of the acceptance criteria (are they testable?
INVEST?) — that is human/judgment work (Foyer B, stays advisory). We only assert
the sections are *present*. The check is content-level (heading match); a section
that is present but empty passes here and is caught downstream by review.

## Enforcement altitude

Content altitudes (`artifact` + `changeset`), mirroring the `governed_files`
pattern of `ADR-205.rego` / `examples/adr/ADR-090.rego`: the `ppg-guard` hook
sends each story write to `/verify_artifact`, and the apply-time backstop sends
the whole diff to `/verify_changeset`. Either way the incomplete story is
rejected before it becomes the Dev agent's source of truth.
