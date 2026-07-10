# Skill governance

Publish-time validation of Claude Code skills (`SKILL.md` + optional
companion `SKILL.rego`), exposed as `POST /validate_skill`. Policies are Rego
files loaded from the directory given by the `-skill-governance` flag
(default `skill-governance/`), all in package `ppg.skills.governance`,
evaluated as a single OPA query over
`data.ppg.skills.governance.violation`. An empty policy directory yields a
permissive linter. Like the plan linter, it fails closed: an undecodable
evaluation result is reported as a violation on field `linter`.

## Request (`POST /validate_skill`)

Mirrors `internal/skill.Skill`. Note the spelling difference between the
YAML front matter of a `SKILL.md` (`argument-hint`) and the JSON API
(`argument_hint`); `skill.Parse` handles the file-side conversion.

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | ✅ | Skill identifier, the `/skill-name` trigger |
| `description` | string | ✅ | Discovery text: what the skill does and when to use it |
| `version` | string | ✅* | Semver, required by policy for registry publication |
| `argument_hint` | string | ❌* | Required by policy when the body uses `$ARGUMENTS` |
| `body` | string | ✅ | Markdown body: the instructions the agent follows |
| `rego_policy` | string | ❌* | Content of the companion `SKILL.rego`; required by policy for tier ≥ 1 skills |

\* enforced by the governance policies below, not by JSON decoding.

## Responses

`200`:

| Field | Type | Description |
|---|---|---|
| `status` | string | `SKILL_VALID` |
| `tier` | int | Security tier (see below) |

`422`:

| Field | Type | Description |
|---|---|---|
| `status` | string | `SKILL_REJECTED` |
| `violations[].field` | string | Skill field that failed (`name`, `description`, `version`, `argument_hint`, `rego_policy`, `linter`) |
| `violations[].message` | string | How to fix it |
| `violations[].nature` | string | `amplifier` \| `compensatory` |
| `guidance` | string | Publication guidance |

## Security tiers

Computed in Go (`skill.Linter.Tier`) from case-sensitive substring matches on
the body — the exact PoC behavior, deliberately naive (production posture: a
deny-by-default tool allowlist):

| Tier | Trigger | Meaning |
|---|---|---|
| 0 | neither `Edit`/`Write` nor `Bash` in the body | Read-only |
| 1 | body contains `Edit` or `Write` | File modifications |
| 2 | body contains `Bash` | Shell access |

## Structural rules (`skill-governance/structure.rego`)

| Field | Rule | nature |
|---|---|---|
| `name` | required | amplifier |
| `name` | `^[a-z][a-z0-9-]{0,31}$` (lowercase kebab-case, ≤32 chars) | amplifier |
| `description` | required | amplifier |
| `description` | ≥ 50 characters | amplifier |
| `description` | ≤ 500 characters | amplifier |
| `description` | starts with a third-person verb (naive check: `^[A-Z][a-z]+s\s`) | amplifier |
| `version` | required | amplifier |
| `argument_hint` | required when the body uses `$ARGUMENTS` | amplifier |
| `body` | ≤ 500 lines | amplifier |
| `body` | no hardcoded secrets (pattern scan: AWS keys, PEM blocks, inline `key = "..."` assignments) | amplifier |

## Security rules (`skill-governance/security.rego`)

| Rule | nature |
|---|---|
| A skill whose body mentions `Edit`, `Write`, or `Bash` (tier ≥ 1) must ship a companion `rego_policy`, so the plan linter can enforce its requirements at `lock_in_plan` time | amplifier |

The verb and secret checks are deliberately naive pattern matches, same
assumed posture as the tier keywords: deterministic and reproducible over
clever ("This skill..." passes the verb check; an obfuscated secret escapes
the scan).
