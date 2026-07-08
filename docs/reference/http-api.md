# HTTP API

> Factual and exhaustive. No pedagogy; see the
> [tutorials](../tutorials/01-first-planning-cycle.md) for that.

## Endpoints

| Method | Route | Body | Success | Error |
|---|---|---|---|---|
| `POST` | `/enrich` | `{intent, repository_context}` | `200` + amplifier context | `400` malformed body |
| `POST` | `/lock_in_plan` | plan (see [plan contract](plan-contract.md)) | `200` `{status: PLAN_LOCKED, plan_hash, execution_ticket}` | `400` `PLAN_MALFORMED` · `422` `PLAN_REJECTED` + `violations[]` |
| `POST` | `/tools/{name}` | `{ticket, targets, payload}` | `200` tool result | `403` `REFUSED` (`TOOL_NOT_IN_PLAN` \| `OUT_OF_PLAN_SCOPE`) · `401` invalid/expired ticket |
| `GET` | `/debt_report` | — | `200` debt report | — |
| `POST` | `/validate_skill` | skill (see [skill governance](skill-governance.md)) | `200` `{status: SKILL_VALID, tier}` | `400` malformed body · `422` `SKILL_REJECTED` + `violations[]` |

## `POST /enrich`

Request:

| Field | Type | Required | Description |
|---|---|---|---|
| `intent` | string | ✅ | Natural-language description of what the agent is about to do |
| `repository_context.name` | string | ✅ | Repository being worked on |
| `repository_context.tech_stack` | string[] | ✅ | Technologies of the repository |

Response (`200`):

| Field | Type | Description |
|---|---|---|
| `status` | string | `CONTEXT_ENRICHED` |
| `amplifier_context.architectural_invariants[]` | object[] | The invariants to inject into the agent's planning context |
| `amplifier_context.architectural_invariants[].adr_id` | string | Source ADR |
| `amplifier_context.architectural_invariants[].invariant` | string | The semantic invariant text (never a recipe) |
| `amplifier_context.source_adrs` | string[] | All matched ADR ids |
| `compensatory_scaffolding[]` | object[] | Matched ADRs tagged `compensatory`, with their `sunset_condition` (the parts of the returned context scheduled to disappear) |

Selection rule: an ADR is returned iff one of its `scope_selectors` appears
(case-insensitive) in the intent. The result is advisory only; enforcement
happens at `/lock_in_plan`.

## `POST /lock_in_plan`

Request: a plan matching the [plan contract](plan-contract.md).

Response (`200`):

| Field | Type | Description |
|---|---|---|
| `status` | string | `PLAN_LOCKED` |
| `plan_hash` | sha256 hex | Canonical fingerprint of the locked plan |
| `execution_ticket` | string | The signed [capability ticket](capability-ticket.md) (JWT) |

Response (`422 PLAN_REJECTED`): `violations[]`, each with `policy_id`,
`message`, and `nature` (`amplifier` \| `compensatory`), plus a `guidance`
string. The linter fails closed: an undecodable policy evaluation result is
reported as a `linter_eval_error` violation, never as a pass.

## `POST /tools/{name}`

Request:

| Field | Type | Required | Description |
|---|---|---|---|
| `ticket` | string | ✅ | The capability ticket from `lock_in_plan` |
| `targets` | string[] | ✅ | Files/resources the tool acts on |
| `payload` | object | ✅ | Tool-specific input (see [policy catalog](policy-catalog.md) for the registered tools) |

Refusal (`403 REFUSED`) carries `code` (`TOOL_NOT_IN_PLAN` \|
`OUT_OF_PLAN_SCOPE`), `attempted`, `allowed`, and `guidance`.

## `GET /debt_report`

Response (`200`): `transition_debt_ratio`, `pending_sunsets[]`, `health`
(`OK` \| `DEBT_ALERT` when the ratio is ≥ 0.3).

## `POST /validate_skill`

Request and response are specified in [skill governance](skill-governance.md).
