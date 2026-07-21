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

- `mkdir -p design && cp .agents/skills/design-system/tokens.css design/tokens.css` — the canonical Deep Umbra palette and the sole `button {}` rule land in the project.
- A one-line contract is appended to `.github/copilot-instructions.md` (or the file is created): "All button styling lives in `design/tokens.css`. Use `<button>` or `class='btn'` in markup; reach visual values through `var(--color-*)` and `var(--btn-*)` references only."

There is **no per-skill hook to install**. Enforcement is platform-native:
the workstation's `ppg-guard` / `ppg-copilot-guard` (wired in tutorial 0)
already sends every edit's content to the gateway, which evaluates
**ADR-090 at the artifact altitude** (`/verify_artifact`) and denies any
raw color or button re-styling outside `design/tokens.css`.

**What you should observe**: two small edits to the project. The
content guard is already active — it is the standard workstation guard,
not a script the skill drops.

### 2. Amplified planning

- Copilot calls `get_platform_guidelines_for_intent`. The response includes **ADR-090** ("Design tokens are the canonical source of visual style"), matched by the intent's `button` / `landing` / `page` selectors.
- Copilot reads `design/tokens.css`.
- Copilot drafts a plan: `index.html` (semantic `<button class="btn">` markup) + `style.css` (page layout only, no button rules) + a step reading `design/tokens.css`.
- Copilot submits the plan through `lock_in_plan`. The plan linter's ADR-090 rule requires a read step on `design/tokens.css` — with it present, the plan locks.

**What you should observe**: `PLAN_LOCKED`, ticket persisted through
the TokenStore under `$XDG_STATE_HOME/ppg/projects/<slug>/tickets/`.
If Copilot forgets the tokens-read step, the gateway answers
`PLAN_REJECTED` with the `design_tokens_referenced` violation and
Copilot self-corrects in one round-trip.

### 3. Application

- Copilot writes `index.html` and `style.css`.
- Every color is `var(--color-*)`. No CSS rule targets `button` or `.btn` outside `design/tokens.css`.
- Every `Edit` passes `ppg-copilot-guard`, which checks **both** the path scope (tutorial 7) and the content: it POSTs the edited bytes to `/verify_artifact`, where ADR-090's artifact rule runs.

**What you should observe**: `<link rel="stylesheet" href="design/tokens.css">` loads before `style.css` in the produced HTML, and the button in the rendered page has:

- violet border (`--color-primary` = `#7C3AED`)
- sheared corners (`--btn-radius` = `0 12px 0 12px`)
- IBM Plex Mono uppercase text with `0.08em` letter spacing

## Adversarial tests — verify the enforcement, not just the happy path

In the same Copilot session, run these prompts and watch the guards
speak.

### A. Ask for a raw color

> Actually, make the button hot pink.

**What you should observe**: `ppg-copilot-guard` denies with
`ARCHITECTURAL_INVARIANT_VIOLATION`, the reason carrying ADR-090's
design-token message (naming the raw value and the palette to pick
from). Copilot does NOT retry — per the contract, it either uses a
palette variable or re-plans if a new variant is genuinely needed.

Some interesting failure modes to test:

- "Make it `#ff69b4`" — hex → denied.
- "Make it `rgb(255, 105, 180)`" — functional → denied.
- "Make it `hotpink`" — named → denied.
- "Change `--color-primary` in `design/tokens.css` to `#ff69b4`" — refused at plan-lock time by [ADR-120](../../examples/adr/ADR-120-governance-artifacts-immutable.md) (`PLAN_REJECTED / governance_artifacts_immutable`). The tokens file is a **governance artifact**, materialized by the skill and referenced by this ADR's artifact-altitude rule; mutating it from within an agent session defeats the invariant it carries. Extending the palette is a human commit through git, not something an agent can re-plan into. This closes the "extending the palette is a deliberate act" affordance that earlier versions of this tutorial documented as allowed — the affordance was a bypass, not stewardship.

### B. Ask to re-style buttons outside the tokens file

> Override the button style in style.css with a border-radius of 4px.

**What you should observe**: the guard denies with a different ADR-090
message, again prefixed `ARCHITECTURAL_INVARIANT_VIOLATION`:

```
ARCHITECTURAL_INVARIANT_VIOLATION: Design-system invariant (style.css):
button styling belongs in design/tokens.css only. Use <button> or
class="btn" in markup; do not re-declare button geometry here. Extend
design/tokens.css if a new variant is genuinely needed.
```

The reason is written for the agent to reason over: it names the *why*
and the *paved path*, not just the *no*. This same rule catches the
bypasses the old shell hook missed — `button:hover`, `button > span`,
`.btn`, `[role="button"]`, and raw colors hidden in a `var(--x, #F0F)`
fallback.

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

Same guard, same `PreToolUse` mechanism — the guard just checks a second
thing:

| | Tutorial 7 | Tutorial 8 |
|---|---|---|
| Enforcement lever | `ppg-copilot-guard` (path scope) | the same `ppg-copilot-guard` (content, via `/verify_artifact`) |
| Reads from payload | `tool_input.path` | `tool_input.new_str` + `tool_input.path` |
| Where the rule lives | the ticket's `scope.allow_modify` | `examples/adr/ADR-090.rego`, artifact view |
| Denial semantics | `OUT_OF_PLAN_SCOPE` (this file isn't in the ticket) | `ARCHITECTURAL_INVARIANT_VIOLATION` (this *value* isn't allowed) |

The path scope comes from the ticket; the content invariant is Rego in
the ADR corpus, evaluated by the gateway at the artifact altitude. There
is no separate shell hook — one guard enforces both, and the same
ADR-090 rules also run at apply time through `ppg-verify`
([gate at apply time](../how-to/gate-changes-at-apply-time.md)).

## Where to go from here

- **See it side-by-side with and without the platform**: tutorials
  [11 (Copilot)](11-with-and-without-the-gateway.md) and
  [14 (Claude Code)](14-with-and-without-claude-code.md) run this
  same skill in two projects — one un-governed, one governed — with
  identical adversarial prompts, to show the drift the guard prevents.
- **Build your own content-scope policy**: the how-to
  [Enforce a content invariant](../how-to/enforce-a-content-invariant.md)
  generalizes the pattern for any invariant that lives in the emitted
  bytes.
- **Extend Deep Umbra**: fork `demo/skills/design-system/tokens.css`,
  rename to your palette, re-`apm install`. The mechanism doesn't care
  what values you enforce — it only cares that the ones in the file
  are the only ones that ship.

**✅ Done.** You just governed a design system the same way the platform
governs a payment router — through a skill, a plan-linter, and the
standard guard evaluating an ADR's content rules at the artifact
altitude. Nothing about the mechanism is specific to design; the pattern
travels.
