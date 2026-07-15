---
name: design-system
description: Applies the Deep Umbra design system to any UI work in the project. Use whenever the user asks to build a landing page, a web component, a page prototype, or any piece of styling in HTML/CSS/TSX. Materializes the canonical palette in design/tokens.css and enforces every subsequent edit against it via a PreToolUse content-scope hook.
version: 1.0.0
argument-hint: "<what to build, e.g. a landing page with a START PAYMENT CTA button>"
---

Build what the user asked for ($ARGUMENTS) through the Platform Planning
Gateway, applying the Deep Umbra design system. Bootstrap runs once per
project; from step 2 onward every edit passes through the design-guard
hook, and any raw color or button re-styling outside `design/tokens.css`
is denied at write time.

## 1. Bootstrap (idempotent — skip lines that already hold)

Run these once per project, in order:

- `chmod +x .agents/skills/design-system/hooks/design-guard.sh`
  (APM installs do not preserve execute bits.)
- If `design/tokens.css` does not exist: `mkdir -p design && cp
  .agents/skills/design-system/tokens.css design/tokens.css`.
- If `.github/hooks/design.json` does not exist: create it as:

  ```json
  {
    "hooks": {
      "PreToolUse": [
        { "type": "command",
          "command": "./.agents/skills/design-system/hooks/design-guard.sh",
          "timeoutSec": 5 }
      ]
    }
  }
  ```

- If `.github/copilot-instructions.md` does not already mention the
  design system, append: "All button styling lives in
  `design/tokens.css`. Use `<button>` or `class='btn'` in markup; do
  not re-style buttons elsewhere. Reach visual values through
  `var(--color-*)` and `var(--btn-*)` references only."

The bootstrap edits happen BEFORE the design-guard is active. From the
next step onward every edit is enforced.

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
- Submit the plan through `lock_in_plan`. If the gateway rejects it,
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

If `design-guard.sh` denies an edit with `DESIGN_SYSTEM_VIOLATION`, do
not retry the same call. Read the reason: it names the disallowed value
and the palette to pick from. Either switch to a `var(--color-*)`
reference, or route the styling through `design/tokens.css` (for a new
button variant, extend the tokens file and re-plan through
`lock_in_plan`).

If `ppg-copilot-guard` denies with `OUT_OF_PLAN_SCOPE`, re-plan through
`lock_in_plan` if the extra change is genuinely needed.
