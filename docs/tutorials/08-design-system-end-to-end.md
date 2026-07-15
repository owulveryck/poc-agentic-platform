# Tutorial 8 — govern a design system through a skill

> **Goal**: build a landing page whose button colors and geometry come
> *only* from a canonical palette, enforced deterministically at edit
> time. Not requested, not linted after the fact — refused before the
> bytes hit disk.
>
> This extends the amplified-loop mechanism from **path-scope**
> invariants (tutorial 7's ADR-070: which files may be modified) to
> **content-scope** invariants: properties of the emitted bytes. The
> pattern generalizes to brand voice, i18n keys, license headers,
> deprecated-API bans — anywhere the invariant lives in the values.
>
> Time: ~15 minutes.
> Prerequisites: [tutorial 0](00-bootstrap.md) completed. Gateway
> running on `:8765`. `apm` installed.

## The four commands

Everything else happens inside the skill invocation.

```bash
mkdir ~/deep-umbra-landing && cd ~/deep-umbra-landing && git init
apm install ~/src/poc-agentic-platform/demo --target copilot
git add -A && git commit -q -m "install design-system skill via APM"
# open ~/deep-umbra-landing in the Copilot app
```

The commit right after `apm install` matters: the Copilot desktop app
creates a per-session git worktree from the last commit, and
uncommitted files (including everything APM just installed) are
invisible in that worktree. If you already opened the folder in
Copilot before committing, close the Copilot session and reopen it —
the worktree is created at session start.

Then, in Copilot Chat:

> `/design-system` Build me a landing page with a hero and a big
> "START PAYMENT" CTA button.

Copilot desktop auto-discovers skills committed under
`.agents/skills/` and exposes each as a slash-command (per the
[APM targets matrix](https://microsoft.github.io/apm/reference/targets-matrix/)
and the [agent-skills spec](https://agent-skills.io/)). Claude Code
uses the same slash-command form on `.claude/skills/`.

Alternative prompt forms that also work — pick whichever fits your
narration:
- **Intent-first** (no mention of the skill name; Copilot's semantic
  matcher finds it from the SKILL.md `description`): *"Build me a
  landing page with a hero and a big START PAYMENT CTA button."*
- **Explicit file reference** (works even if discovery fails for any
  reason): *"Follow the workflow in
  `.agents/skills/design-system/SKILL.md`."*

## What just happened, step by step

The `design-system` skill executes its workflow. Watch the following in
the Copilot conversation:

### 1. Bootstrap (idempotent, runs on first invocation only)

The skill's first phase materializes itself in the project:

- `chmod +x .agents/skills/design-system/hooks/design-guard.sh` (defensive; some APM versions strip execute bits).
- `mkdir -p design && cp .agents/skills/design-system/tokens.css design/tokens.css` — the canonical Deep Umbra palette and the sole `button {}` rule land in the project.
- `.github/hooks/design.json` is written, registering `design-guard.sh` as a PreToolUse hook. From the next edit onward, every write is enforced.
- A one-line contract is appended to `.github/copilot-instructions.md` (or the file is created): "All button styling lives in `design/tokens.css`. Use `<button>` or `class='btn'` in markup; reach visual values through `var(--color-*)` and `var(--btn-*)` references only."

**What you should observe**: four small edits to the project, all
passing (no guard active yet). After this phase, the guard is loaded.

### 2. Amplified planning

- Copilot calls `get_platform_guidelines_for_intent`. The response includes **ADR-090** ("Design tokens are the canonical source of visual style"), matched by the intent's `button` / `landing` / `page` selectors.
- Copilot reads `design/tokens.css`.
- Copilot drafts a plan: `index.html` (semantic `<button class="btn">` markup) + `style.css` (page layout only, no button rules) + a step reading `design/tokens.css`.
- Copilot submits the plan through `lock_in_plan`. The plan linter's ADR-090 rule requires a read step on `design/tokens.css` — with it present, the plan locks.

**What you should observe**: `PLAN_LOCKED`, ticket lands in
`.ppg-ticket`. If Copilot forgets the tokens-read step, the gateway
answers `PLAN_REJECTED` with the `design_tokens_referenced` violation
and Copilot self-corrects in one round-trip.

### 3. Application

- Copilot writes `index.html` and `style.css`.
- Every color is `var(--color-*)`. No CSS rule targets `button` or `.btn` outside `design/tokens.css`.
- Every `Edit` passes both `ppg-copilot-guard` (path scope, tutorial 7) and `design-guard.sh` (content scope, this tutorial).

**What you should observe**: `<link rel="stylesheet" href="design/tokens.css">` loads before `style.css` in the produced HTML, and the button in the rendered page has:

- violet border (`--color-primary` = `#7C3AED`)
- sheared corners (`--btn-radius` = `0 12px 0 12px`)
- IBM Plex Mono uppercase text with `0.08em` letter spacing

## Adversarial tests — verify the enforcement, not just the happy path

In the same Copilot session, run these prompts and watch the guards
speak.

### A. Ask for a raw color

> Actually, make the button hot pink.

**What you should observe**: `design-guard.sh` denies with
`DESIGN_SYSTEM_VIOLATION`, naming the closest allowed tokens. Copilot
does NOT retry — per the contract, it either uses a palette variable
or re-plans if a new variant is genuinely needed.

Some interesting failure modes to test:

- "Make it `#ff69b4`" — hex → denied.
- "Make it `rgb(255, 105, 180)`" — functional → denied.
- "Make it `hotpink`" — named → denied.
- "Change `--color-primary` in `design/tokens.css` to `#ff69b4`" — allowed (the tokens file is the one place raw values live). This is where design system stewardship lives; extending the palette is a deliberate act.

### B. Ask to re-style buttons outside the tokens file

> Override the button style in style.css with a border-radius of 4px.

**What you should observe**: `design-guard.sh` denies with a different
reason:

```
DESIGN_SYSTEM_VIOLATION: button styling belongs in design/tokens.css
only. The design system's <button> rule is canonical — use <button> or
class="btn" in markup, do not re-style buttons in style.css. If a new
button variant is genuinely needed, extend design/tokens.css.
```

The reason is written for the agent to reason over: it names the *why*
and the *paved path*, not just the *no*.

## Final verification — deterministic, reader-runnable

You do not have to trust Copilot's account of what it did. Verify:

```bash
# No raw colors anywhere except in the canonical tokens file
grep -REn '#[0-9a-fA-F]{3,8}|rgb\(|hsl\(' index.html style.css

# No button rule outside tokens.css
grep -REn '(^|[^-a-zA-Z0-9])(button|\.btn)[[:space:]]*[{,]' index.html style.css
```

Both commands return no matches. The design system holds, not because
the model was polite, but because the bytes couldn't get past the guard.

## What makes this different from tutorial 7

Same PreToolUse mechanism, different granularity:

| | Tutorial 7 | Tutorial 8 |
|---|---|---|
| Enforcement lever | `ppg-copilot-guard` (path-scope) | `design-guard.sh` (content-scope) |
| Reads from payload | `tool_input.path` | `tool_input.new_str` + `tool_input.path` |
| Denial semantics | `OUT_OF_PLAN_SCOPE` (this file isn't in the ticket) | `DESIGN_SYSTEM_VIOLATION` (this *value* isn't allowed) |
| Language | Go binary | Shell script |
| Location | `adapters/copilot/guard/` | Inside the skill (`demo/skills/design-system/hooks/`) |

Both hooks fire on `PreToolUse`; most-restrictive `deny` wins. They
compose without knowing about each other — that's the mechanism's
generality.

## Where to go from here

- **Build your own content-scope policy**: the how-to
  [Enforce a content invariant](../how-to/enforce-a-content-invariant.md)
  generalizes the pattern for any invariant that lives in the emitted
  bytes.
- **Extend Deep Umbra**: fork `demo/skills/design-system/tokens.css`,
  rename to your palette, re-`apm install`. The mechanism doesn't care
  what values you enforce — it only cares that the ones in the file
  are the only ones that ship.

**✅ Done.** You just governed a design system the same way the platform
governs a payment router — through a skill, a plan-linter, and a
content-scope PreToolUse hook. Nothing about the mechanism is specific
to design; the pattern travels.
