# ADR front matter

Every file matching `adr/*.md` is loaded at gateway startup. The YAML front
matter drives retrieval (`/enrich`) and enforcement (`/lock_in_plan`); the
Markdown body after the front matter is the semantic invariant text injected
into the agent's planning context.

| Field | Type | Required | Values |
|---|---|---|---|
| `adr_id` | string | ✅ | `^ADR-[0-9]+$` |
| `title` | string | ✅ | |
| `status` | enum | ✅ | `proposed` \| `accepted` \| `deprecated` \| `superseded` |
| `nature` | enum | ✅ | `amplifier` \| `compensatory` |
| `sunset_condition` | string \| null | ✅ | `null` iff `amplifier` |
| `scope_selectors` | string[] | ✅ | Trigger keywords, matched case-insensitively against the intent |
| `enforcement.mode` | enum | ❌ | `declarative` \| `programmatic` |
| `enforcement.policy_id` | string | ❌ | Policy identifier; populates the linter `Registry` (and the debt report) |
| `enforcement.rego` | string | ❌ | Filename of the paired Rego policy, relative to the ADR directory (e.g. `ADR-060.rego`). Only for `programmatic` mode; loaded into the plan linter at startup |

A declarative-only ADR (like `ADR-042`) sets neither `enforcement.rego` nor a
Rego file: its semantic directive is injected at `/enrich` but nothing is
checked at `/lock_in_plan`. A programmatic ADR is a dual-representation
artifact: Markdown body for planning, `.rego` file for deterministic
enforcement (see the [policy catalog](policy-catalog.md)).
