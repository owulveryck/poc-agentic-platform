---
adr_id: ADR-120
title: Governance artifacts are immutable from within agent sessions
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["design", "tokens", "palette", "skill", "governance", "css", "ui", "styling"]
enforcement:
  mode: programmatic
  policy_id: governance_artifacts_immutable
  rego: ADR-120.rego
  altitudes: [plan]
---

## Invariant

The following paths constitute the **governance surface** and MUST NOT
appear as write/edit targets in any plan locked through the validation
server:

- `design/tokens.css` — the canonical design-system file materialized
  by the `design-system` skill and referenced by ADR-090's artifact
  rule.
- `.claude/skills/**` — Claude Code skill definitions (the `SKILL.md`
  bodies the agent executes under, plus companion assets).
- `.agents/skills/**` — the cross-agent skill directory per the
  [agent-skills spec](https://agent-skills.io/), consumed by Copilot
  desktop and other agents reading the standard location.

Read access is always permitted — a plan may (and often must) include
a Read step targeting `design/tokens.css` per ADR-090's own plan-view
rule. Only mutations are refused.

## Rationale (durability)

Without this invariant, ADR-090's design-token contract can be
defeated by two lines of prompt: "make the button hot pink" →
model re-plans with a `Write` step targeting `design/tokens.css`
(which ADR-090 exempts from its artifact-altitude check because
that file is the one place raw values legitimately live) → the plan
locks, the ticket issues, the write succeeds, the palette bends.

The same argument extends to the skill body itself. If an agent
running under `.claude/skills/design-system/SKILL.md` can `Edit`
that file mid-session, the workflow it is supposed to follow is
whatever the last edit says it is. Skills stop being contracts and
become suggestions.

The invariant carries whatever the intelligence of the model, so it
is an AMPLIFIER — no sunset condition. A smarter model does not
help here: the smarter it is, the more elegantly it will route
around the artifact-altitude check by editing the tokens file
directly. The defence must live at a level the agent cannot rewrite
from within its own loop.

The paved path for legitimate extension of these artifacts (a new
palette variant, a new step in a skill, a new skill entirely) is a
**human commit** through git, outside any agent session. The validation server
governs the agent loop; git governs the source of truth. This
invariant is what keeps the two layers separate.

## What we do NOT write here

We do not enumerate individual skills. The rule is prefix-based
(`.claude/skills/`, `.agents/skills/`) so any skill installed via
APM or hand-authored is covered automatically. The one exact-match
entry is `design/tokens.css` because ADR-090 references it by name;
if the design system moves to a different canonical path, both ADRs
update together.

We do not attempt to cover the ADR corpus itself
(`examples/adr/**`). The ADR path is CLI-configurable via the `-adr`
flag on `ppg`; different deployments put it in different places.
Adding a generic "no self-modification of policies" rule requires a
mechanism (declared-adr-path, an env var) that does not yet exist.
Deferred.

## Enforcement stack

Single lever: plan altitude only.

- **Plan altitude** (`input.view == "plan"`): the plan linter reads
  every `step.tool` + `step.targets`. If any write-class tool
  (see below) targets a governance path (exact match on
  `design/tokens.css`, or prefix match on the skill directories),
  the plan is rejected with `PLAN_REJECTED` /
  `governance_artifacts_immutable`. No capability ticket is minted.

No artifact-altitude or changeset-altitude rule is needed. Because
the ticket is never issued, `ppg-guard`'s existing empty-ticket
path (`No capability ticket for this session`) refuses any
subsequent `Write`/`Edit` on the same session, covering the case
where the model tries to skip the plan lock and write directly.
Same holds for `ppg-copilot-guard`. This is why the fix requires
no adapter code changes.

The write-tool list mirrors `isWriteTool` in
`adapters/claudecode/guard/main.go`: `Write`, `Edit`, `MultiEdit`,
`NotebookEdit`, `Update`, `create_file`, `edit_file`, `editFiles`,
`str_replace_editor`, `apply_patch`, `patch_code`, plus any tool
name containing `Write` or `Edit`. When that list changes, this
Rego rule must be updated to match.

## Known limits

- **Bash-based mutations** (`sed -i design/tokens.css`,
  `> design/tokens.css`, `cp somewhere design/tokens.css`) bypass
  this rule. `Bash` is intentionally not a write-class tool — it is
  used for tests, builds, and real work, and treating every Bash
  call as an edit would make the platform unusable. The apply-time
  backstop is `ppg-verify`: it runs the full ADR corpus against the
  diff before a commit reaches shared main. A Bash-based mutation
  of a governance artifact is caught there, not here.
- **Rename or symlink tricks** (`mv design/tokens.css design/x.css`
  then `Write design/x.css`) evade the exact-match rule. The
  apply-time backstop catches this as well; the plan-altitude
  rule intentionally stays simple.
- **The write-tool list drift** — if a new adapter introduces a
  write tool with an unfamiliar name and it is not added here, that
  adapter's writes to governance paths would pass the plan linter.
  Mitigation: the `contains(t, "Write")` / `contains(t, "Edit")`
  catch-all in the Rego covers most naming conventions, and
  `ppg-verify` remains the backstop.
