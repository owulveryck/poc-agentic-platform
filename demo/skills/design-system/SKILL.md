---
name: design-system
description: Applies the Deep Umbra design system to any UI work in the project. Use whenever the user asks to build a landing page, a web component, a page prototype, or any piece of styling in HTML/CSS/TSX. Materializes the canonical palette in design/tokens.css; every subsequent edit is enforced against ADR-090 by the platform guard at write time.
version: 1.0.0
argument-hint: "<what to build, e.g. a landing page with a START PAYMENT CTA button>"
---

Build what the user asked for ($ARGUMENTS) through the governance
harness, applying the Deep Umbra design system. Enforcement is platform-native:
the workstation's `ppg-guard` / `ppg-copilot-guard` sends every edit's content to
the validation server, which evaluates ADR-090 at the **artifact altitude** and denies any
raw color or button re-styling outside `design/tokens.css`. There is no
skill-specific hook to install — the governed workstation (tutorial 0) already
wires the guard.

## 1. Bootstrap (idempotent — skip lines that already hold)

Run these once per project, in order:

- If `design/tokens.css` does not exist: `mkdir -p design && cp
  .agents/skills/design-system/tokens.css design/tokens.css`.
- If `.github/copilot-instructions.md` does not already mention the
  design system, append: "All button styling lives in
  `design/tokens.css`. Use `<button>` or `class='btn'` in markup; do
  not re-style buttons elsewhere. Reach visual values through
  `var(--color-*)` and `var(--btn-*)` references only."

## 2. Amplified planning

- Call `get_platform_guidelines_for_intent` with the intent (a
  paraphrase of "$ARGUMENTS") and the repository context. Read every
  returned invariant — ADR-090 (design tokens are canonical) will
  appear.
- Read `design/tokens.css` — it is the only source of truth for visual
  values.
- Draft the plan: markup files under `./` (or the project's chosen web
  root) using semantic `<button>` and `class="btn"`; a page-level
  stylesheet whose steps include `design/tokens.css` as a read target
  and never re-declare button rules. Include a `link` step (or
  equivalent) so `design/tokens.css` loads before other stylesheets.
- Submit the plan through `lock_in_plan`. If the validation server rejects it,
  the violation message names the exact criterion — fix precisely that
  and resubmit.

## 3. Application

- Edit each in-scope file with `Edit` or `Write`. Every color is a
  `var(--color-*)` reference; no `<style>` block or CSS rule targets
  `button`, `.btn`, or `[role="button"]` outside `design/tokens.css`.
- Verify the result: `grep -REn '#[0-9a-fA-F]{3,8}|rgb\(|hsl\('
  index.html style.css` returns no matches; `grep -REn
  '(^|[^-a-zA-Z0-9])(button|\.btn)\s*[{,]' index.html style.css`
  also returns no matches (button styling lives in tokens.css only).

## 4. On refusal

If the guard denies an edit citing the ADR-090 design-token invariant, do
not retry the same call. Read the reason: it names the disallowed value
and the palette to pick from, then switch to a `var(--color-*)` reference.
`design/tokens.css` itself is **immutable from within a session**
(`design_tokens_immutable`): a new button variant or palette extension is
a human git commit outside any agent session — tell the user so, do not
re-plan a write to the tokens file.

If the guard denies with `OUT_OF_PLAN_SCOPE`, re-plan through
`lock_in_plan` if the extra change is genuinely needed.
