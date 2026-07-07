# Reference — technical information

> Factual and exhaustive. No pedagogy — see the [tutorial](tutorial.md) for
> that.

## Endpoints

| Method | Route | Body | Success | Error |
|---|---|---|---|---|
| `POST` | `/enrich` | `{intent, repository_context}` | `200` + amplifier context | `400` malformed body |
| `POST` | `/lock_in_plan` | plan (see `schemas/plan.schema.json`) | `200` `{status: PLAN_LOCKED, plan_hash, execution_ticket}` | `400` `PLAN_MALFORMED` · `422` `PLAN_REJECTED` + `violations[]` |
| `POST` | `/tools/{name}` | `{ticket, targets, payload}` | `200` tool result | `403` `REFUSED` (`TOOL_NOT_IN_PLAN` \| `OUT_OF_PLAN_SCOPE`) · `401` invalid/expired ticket |
| `GET` | `/debt_report` | — | `200` debt report | — |

## Plan contract (`schemas/plan.schema.json`)

| Field | Type | Required | Description |
|---|---|---|---|
| `session_id` | uuid | ✅ | Session identifier |
| `intent` | string (≥5) | ✅ | Natural-language intent |
| `repository_context.name` | string | ✅ | Repository name |
| `repository_context.tech_stack` | string[] | ✅ | Detected stacks |
| `steps[].id` | string | ✅ | Step identifier |
| `steps[].action` | string | ✅ | Human-readable action |
| `steps[].tool` | string | ✅ | Platform tool invoked |
| `steps[].targets` | string[] | ✅ | Files/resources touched |
| `steps[].depends_on` | string[] | ❌ | Graph dependencies |

## ADR front matter

| Field | Type | Required | Values |
|---|---|---|---|
| `adr_id` | string | ✅ | `^ADR-[0-9]+$` |
| `title` | string | ✅ | |
| `status` | enum | ✅ | `proposed` \| `accepted` \| `deprecated` \| `superseded` |
| `nature` | enum | ✅ | `amplifier` \| `compensatory` |
| `sunset_condition` | string \| null | ✅ | `null` iff `amplifier` |
| `scope_selectors` | string[] | ✅ | Trigger keywords |
| `enforcement.mode` | enum | ❌ | `declarative` \| `programmatic` |
| `enforcement.policy_id` | string | ❌ | Reference to the linter `Registry` |

## Capability ticket (JWT, HS256 — PoC only)

| Claim | Type | Description |
|---|---|---|
| `iat` / `exp` | int | Issued at / expiry (TTL = 15 min) |
| `session_id` | string | Originating session |
| `plan_hash` | sha256 hex | Canonical fingerprint of the locked plan |
| `scope.allow_modify` | string[] | Files the agent may modify |
| `scope.allow_tool` | string[] | Tools the agent may invoke |

## Policy catalog (`internal/linter.Registry`)

| `policy_id` | nature | sunset condition |
|---|---|---|
| `go_tests_present` | amplifier | — |
| `db_migration_precedes_code` | amplifier | — |
| `external_call_via_proxy` | amplifier (declarative, via ADR-042) | — |
| `explicit_frozen_files_enumeration` | compensatory | model honors `@deprecated` >95% |

Plus one tagged translator: `generic_raw_to_json_translator` (compensatory,
sunset: model reads raw stack traces reliably >95%).

## Statuses and error codes

| Code | Meaning |
|---|---|
| `CONTEXT_ENRICHED` | Enrichment succeeded |
| `PLAN_MALFORMED` | Structural contract violated (400) |
| `PLAN_REJECTED` | Policy violations, `violations[]` provided (422) |
| `PLAN_LOCKED` | Plan valid, ticket issued |
| `TOOL_NOT_IN_PLAN` | Tool absent from the ticket scope (403) |
| `OUT_OF_PLAN_SCOPE` | Target outside the allowed files (403) |
| `EXECUTION_FAILED` | Application failure — see `error_category` |
| `GO_SYNTAX_ERROR` | Patched content does not parse; guidance provided |
| `DATABASE_SCHEMA_CONFLICT` | Schema conflict; staging state provided |
| `health: DEBT_ALERT` | Compensatory ratio ≥ 0.3 |

## Dependencies

`github.com/golang-jwt/jwt/v5`, `gopkg.in/yaml.v3`. No OPA binary required —
see [explanation.md](explanation.md#why-plain-go-policies) for the
production path.
