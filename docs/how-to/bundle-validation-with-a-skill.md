# How to bundle validation with a skill (SKILL.md + SKILL.rego)

> Solves one problem: turning a bare capability into a **skill with
> validation** — one installable package where the human-facing rule
> (`SKILL.md`) travels with its deterministic enforcement (`SKILL.rego`).
> Once installed on a governed workstation, the validation applies with
> zero per-project configuration. This is the shape
> [tutorial 15](../tutorials/15-skill-only-enforcement.md) demonstrates
> end-to-end; this page is the authoring recipe.

## The package layout

```
my-skill/
├── SKILL.md      # the invariant, in prose — what the model reads
└── SKILL.rego    # the policy — what the validation server enforces
```

`SKILL.md` needs front matter (`name`, `description`, `version`; see the
[skill governance reference](../reference/skill-governance.md) for the
publish-gate rules). `SKILL.rego` is optional for read-only (tier-0)
skills and **required** for any skill that instructs file modifications
(tier ≥ 1) — the publish gate refuses a tier ≥ 1 skill without one.

## The Rego contract

Three requirements, nothing else:

1. **A package** under any path you own — the convention is
   `ppg.skills.<name>`:

   ```rego
   package ppg.skills.my_skill

   import rego.v1
   ```

2. **Rules named `violation contains v`** producing objects with
   `policy_id`, `message`, and `nature` (`amplifier` or `compensatory`):

   ```rego
   violation contains v if {
       # ... conditions ...
       v := {
           "policy_id": "my_rule",
           "message":   "Agent-facing explanation of what to change.",
           "nature":    "amplifier",
       }
   }
   ```

3. **A view guard on every rule** — `input.view` is `"plan"`,
   `"artifact"`, or `"changeset"`. A rule without a guard fires at every
   altitude against input shapes it does not expect (it usually silently
   no-ops, which is worse than failing). See the
   [policy views reference](../reference/policy-views.md) for the three
   input schemas.

Policies must be deterministic **by construction**: the engine is
compiled without OPA's nondeterministic built-ins, so `http.send`,
`time.now_ns`, or `rand.intn` in a SKILL.rego fail at registration time
(`422 SKILL_COMPILE_ERROR`).

## Which view for which rule

This is the design decision that matters:

| Rule kind | View | Applies to | Example |
|---|---|---|---|
| **Workflow requirement** — only meaningful when the skill runs | `plan` | Plans that declare this `skill_id` (fail-closed on unknown ids) | "the plan must include a step reading `design/tokens.css`" |
| **Content invariant** — must hold, period | `artifact` + `changeset` | **Every** edit and diff in the session, whether or not the plan declared the skill (union semantics) | "no raw hex colors in UI files", "the palette file is immutable in-session" |

Rules of thumb: requirements about *plan shape* (a step must exist, an
ordering must hold, a file must not be targeted) go in the plan view;
requirements about *produced bytes* go in the artifact view, and the same
rule set should also cover the changeset view — use the `governed_files`
idiom so one set of conditions serves both content altitudes:

```rego
governed_files contains f if {
    input.view == "artifact"
    f := input.artifact
}

governed_files contains f if {
    input.view == "changeset"
    some file in input.changeset.files
    f := file
}
```

The reference implementation is
[`demo/skills/design-system/SKILL.rego`](../../demo/skills/design-system/SKILL.rego):
one plan-view requirement (read the tokens file), one plan-view
prohibition (don't overwrite it), and content-view invariants (raw-hex
ban + in-session palette immutability) via `governed_files`. Copy its
shape.

## Validate before publishing (Gate 1)

```bash
python3 - <<'PY' | curl -sf -X POST http://localhost:8765/validate_skill \
    -H 'Content-Type: application/json' -d @- | jq
import json, pathlib
d = pathlib.Path("my-skill")
print(json.dumps({
    "name": "my-skill",
    "body": (d / "SKILL.md").read_text(),
    "rego_policy": (d / "SKILL.rego").read_text(),
}))
PY
```

`SKILL_VALID` + a tier, or `SKILL_REJECTED` + the violations. The gate
also **compiles** the companion: broken or nondeterministic Rego is
refused at publish time, not discovered at gateway startup. Wire the same
call into CI per
[gate skill publication in CI](gate-skill-publication-in-ci.md).

## Install and watch it enforce

Package the skill for APM (see [`demo/apm.yml`](../../demo/apm.yml) for a
manifest) and install it anywhere the harness scans:

- `~/.claude/skills/` — user-wide, every project on the machine;
- `<project>/.claude/skills/` — project-local (wins on a name collision);
- `<project>/.agents/skills/` — the cross-agent location.

On the next `lock_in_plan`, the MCP server auto-uploads the package to
the validation server (`POST /register_skill`) — no flag, no restart, no
per-project config. From that moment its plan-view rules gate plans that
declare the skill, and its content rules gate **every** edit in the
session. Verify with the tutorial-15 adversarial prompt: ask for a hot
pink button and read the refusal.

## Related

- [Tutorial 15 — skill-only enforcement](../tutorials/15-skill-only-enforcement.md):
  the full demo, zero ADRs.
- [Policy views](../reference/policy-views.md): input schemas, sourcing
  tiers, union semantics, failure modes.
- [Rego survival kit](rego-survival-kit.md): the language subset you need.
- [Skill governance reference](../reference/skill-governance.md): the
  publish-gate rules and security tiers.
