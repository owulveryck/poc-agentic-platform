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

Two levers act together on this invariant:

- Plan-linter (this ADR's `.rego`): any locked plan touching UI files
  must include a step reading `design/tokens.css` — the model must
  acknowledge the tokens exist before planning changes.
- Content-scope PreToolUse hook (`design-guard.sh` in the
  `design-system` skill): denies the edit if the emitted bytes contain
  raw colors or a button rule outside the tokens file.
