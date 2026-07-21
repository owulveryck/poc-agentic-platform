---
adr_id: ADR-090
title: Design tokens are the canonical source of visual style
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["button", "component", "ui", "styling", "css", "design", "landing", "page"]
enforcement:
  mode: programmatic
  policy_id: design_tokens_referenced
  rego: ADR-090.rego
  altitudes: [plan, artifact]
---

## Invariant

Every markup or stylesheet change (`.html`, `.css`, `.tsx`, `.jsx`,
`.svelte`, `.vue`) MUST reach visual values through the design tokens
declared in `design/tokens.css`. Concretely:

- Colors: only `var(--color-*)` references or CSS keywords
  (`transparent`, `inherit`, `currentColor`, `unset`, `initial`).
  Raw hex, `rgb()`, `hsl()`, and named colors are forbidden outside
  the tokens file itself.
- Button geometry (radius, padding, typography, spacing, transition):
  the canonical `button` rule lives in `design/tokens.css`. Markup uses
  `<button>` or `class="btn"`; no page-level or component-level stylesheet
  re-declares button styling.

## Rationale (durability)

This invariant stays true whatever the intelligence of the model: it is
a design-system contract, not a workaround for an LLM limitation. A
smarter model reads the tokens file and reasons over it; a weaker model
still cannot ship a raw `#F0F` because the enforcement is deterministic
at the moment of the edit. Hence AMPLIFIER — no sunset condition.

## What we do NOT write here

We do not enumerate the tokens (they live in `design/tokens.css`,
canonical), and we do not specify a rendering framework. The invariant
is orthogonal to whether the project ships static HTML, React, or
Svelte — the tokens are the interface.

## Enforcement stack

Two levers act together on this invariant, and — since this ADR declares
`altitudes: [plan, artifact]` — both are now the **same** Rego policy
(`ADR-090.rego`), evaluated by the platform at two altitudes:

- Plan altitude (`input.view == "plan"`): any locked plan touching UI files
  must include a step reading `design/tokens.css` — the model must
  acknowledge the tokens exist before planning changes.
- Artifact altitude (`input.view == "artifact"`): the `ppg-guard` /
  `ppg-copilot-guard` PreToolUse hook sends each edit's content to the validation server
  (`POST /verify_artifact`), which runs this same policy against the emitted
  bytes and denies raw colors (including inside `var()` fallbacks) or a button
  rule outside the tokens file. This replaces the earlier bespoke
  `design-guard.sh` shell hook, closing its bypasses.

The artifact-altitude exemption for `design/tokens.css` (the one place
raw values legitimately live) is safe **only because ADR-120 forbids
agent-side writes to that file at plan-lock time**. Without ADR-120, an
adversarial prompt would route around this ADR by re-planning a `Write`
step targeting `design/tokens.css`: the plan would lock (this ADR's
plan rule requires only a *read* of tokens), the ticket would issue,
the write would succeed, and the palette would bend. See
[ADR-120](ADR-120-governance-artifacts-immutable.md) for the closing
argument.
