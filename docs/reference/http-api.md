# HTTP API

> Factual and exhaustive. No pedagogy; see the
> [tutorials](../tutorials/01-first-planning-cycle.md) for that.

## Endpoints

| Method | Route | Body | Success | Error |
|---|---|---|---|---|
| `POST` | `/enrich` | `{intent, repository_context}` | `200` + amplifier context | `400` malformed body |
| `POST` | `/lock_in_plan` | plan (see [plan contract](plan-contract.md)) | `200` `{status: PLAN_LOCKED, plan_hash, execution_ticket}` | `400` `PLAN_MALFORMED` · `422` `PLAN_REJECTED` + `violations[]` |
| `POST` | `/tools/{name}` | `{ticket, targets, payload}` | `200` tool result | `403` `REFUSED` (`TOOL_NOT_IN_PLAN` \| `OUT_OF_PLAN_SCOPE`) · `401` invalid/expired ticket |
| `POST` | `/verify_artifact` | `{ticket, path, content, op?}` | `200` `{status: ARTIFACT_OK}` | `422` `ARTIFACT_REJECTED` + `violations[]` · `403` `REFUSED` (path out of scope) · `401` invalid/expired ticket · `400` malformed body |
| `POST` | `/verify_changeset` | `{ticket, files[], plan_hash?}` | `200` `{status: CHANGESET_OK}` | `422` `CHANGESET_REJECTED` + `violations[]` · `409` `PLAN_SUBSTITUTION` · `403` `REFUSED` · `401` invalid/expired ticket · `400` malformed body |
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

The Smart Tool also evaluates the artifact-view policy over the payload content
(`content` or `statement`): if the content breaks an invariant, it returns
`200` with `{status: EXECUTION_FAILED, exit_code: 1, error_category:
ARCHITECTURAL_INVARIANT_VIOLATION, message, remediation_guidance:{violations[],
allowed_actions[]}}` and does **not** run the tool.

## Policy at three altitudes

The same Rego corpus (`data.ppg.linter.violation`) is evaluated at three
altitudes, discriminated by the `input.view` field:

| View | Endpoint | Input the rules read | When |
|---|---|---|---|
| `plan` | `/lock_in_plan` | `input.steps` (the proposed plan) | lock time |
| `artifact` | `/verify_artifact` | `input.artifact` — `{path, content, op}` | in-loop, one edit |
| `changeset` | `/verify_changeset` | `input.changeset.files` — a list of `{path, content, op}` | apply time, the whole diff |

An ADR's `.rego` opts into an altitude with a `violation` rule guarded by
`input.view == "…"`; the altitudes it implements are declared in its
[front matter](adr-front-matter.md) (`enforcement.altitudes`).

## `POST /verify_artifact`

The in-loop check the guards (`ppg-guard`, `ppg-copilot-guard`) and Smart Tools
call: it evaluates the artifact-view policy against one edited file's actual
content. The ticket and path scope are verified first, then the content policy.

Request:

| Field | Type | Required | Description |
|---|---|---|---|
| `ticket` | string | ✅ | The capability ticket from `lock_in_plan` |
| `path` | string | ✅ | Project-relative path of the edited file |
| `content` | string | ✅ | The proposed content to write |
| `op` | string | ❌ | Operation hint (e.g. `write`), passed to the rules as `input.artifact.op` |

Responses:

| Status | Body |
|---|---|
| `200` | `{status: ARTIFACT_OK}` |
| `422` | `{status: ARTIFACT_REJECTED, violations[], guidance}` — the file scope is allowed but the content breaks an invariant |
| `403` | `{status: REFUSED, code, attempted, allowed, guidance}` — the path is outside the ticket scope |
| `401` | `{error}` — invalid or expired ticket |
| `400` | `{error}` — malformed body |

## `POST /verify_changeset`

The apply-time backstop (`ppg-verify`, CI): it evaluates the changeset-view
policy against a whole diff. It verifies the ticket, that every changed path is
in scope, and — when `plan_hash` is supplied — that the plan being executed
still matches the one the ticket was issued for.

Request:

| Field | Type | Required | Description |
|---|---|---|---|
| `ticket` | string | ✅ | The capability ticket from `lock_in_plan` |
| `files` | object[] | ✅ | The changed files, each `{path, content, op?}` |
| `plan_hash` | sha256 hex | ❌ | When set, checked against the ticket's `plan_hash` claim |

Responses:

| Status | Body |
|---|---|
| `200` | `{status: CHANGESET_OK}` |
| `422` | `{status: CHANGESET_REJECTED, violations[], guidance}` |
| `409` | `{status: PLAN_SUBSTITUTION, expected, got, guidance}` — supplied `plan_hash` ≠ the ticket's `plan_hash` claim |
| `403` | `{status: REFUSED, code, attempted, allowed, guidance}` — a changed path is outside the ticket scope |
| `401` | `{error}` — invalid or expired ticket |
| `400` | `{error}` — malformed body |

## `GET /debt_report`

Response (`200`): `transition_debt_ratio`, `pending_sunsets[]`, `health`
(`OK` \| `DEBT_ALERT` when the ratio is ≥ 0.3).

## `POST /validate_skill`

Request and response are specified in [skill governance](skill-governance.md).
