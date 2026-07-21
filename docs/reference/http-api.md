# HTTP API

> Factual and exhaustive. No pedagogy; see the
> [tutorials](../tutorials/01-first-planning-cycle.md) for that.

## Endpoints

| Method | Route | Body | Success | Error |
|---|---|---|---|---|
| `POST` | `/enrich` | `{intent, repository_context}` | `200` + amplifier context | `400` malformed body |
| `POST` | `/lock_in_plan` | plan (see [plan contract](plan-contract.md)) | `200` `{status: PLAN_LOCKED, plan_hash, execution_ticket}` | `400` `PLAN_MALFORMED` · `422` `PLAN_REJECTED` + `violations[]` · `409` `POLICY_CONFLICT` (livelock escalation) |
| `POST` | `/register_skill` | `{session_id, name, skill_md, skill_rego?}` | `200` `{status: SKILL_REGISTERED}` | `400` malformed body · `422` `SKILL_COMPILE_ERROR` + `error` |
| `POST` | `/tools/{name}` | `{ticket, targets, payload}` | `200` tool result | `403` `REFUSED` (`TOOL_NOT_IN_PLAN` \| `OUT_OF_PLAN_SCOPE`) · `401` invalid/expired ticket |
| `POST` | `/verify_artifact` | `{ticket, path, content, op?}` | `200` `{status: ARTIFACT_OK}` | `422` `ARTIFACT_REJECTED` + `violations[]` · `403` `REFUSED` (path out of scope) · `401` invalid/expired ticket · `400` malformed body |
| `POST` | `/verify_changeset` | `{ticket, files[], plan_hash?}` | `200` `{status: CHANGESET_OK}` | `422` `CHANGESET_REJECTED` + `violations[]` · `409` `PLAN_SUBSTITUTION` · `403` `REFUSED` · `401` invalid/expired ticket · `400` malformed body |
| `POST` | `/discover_service` | `{capability?, intent?, repository_context?}` | `200` `{status: SERVICE_FOUND, recommended, alternatives[], policy_notes[]}` | `200` `NO_SERVICE_FOR_CAPABILITY` · `503` `SERVICE_CATALOG_UNAVAILABLE` · `400` malformed body |
| `GET` | `/services` | — | `200` `{services[]}` | — |
| `GET` | `/services/{id}` | — | `200` service record | `404` `SERVICE_NOT_FOUND` |
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

Response (`409 POLICY_CONFLICT`) — the **livelock escalation**. When a
session's plans are rejected 3 consecutive times with a byte-identical
violation policy-id set, "fix and resubmit" stops being honest guidance:
either the policies are mutually unsatisfiable for this intent, or the
required plan shape is unreachable from the agent's approach. The gateway
switches to a hard block carrying:

| Field | Description |
|---|---|
| `policy_ids` | The sorted, deduplicated ids of the stable violation set |
| `policy_sources` | Per id: `adr` (ADR corpus), `skill` (a companion SKILL.rego), or `built-in` (linter rule) — who must be in the room |
| `consecutive_rejections` | The streak length |
| `escalation_log` | Path of the append-only JSONL record written for the platform team (`$XDG_STATE_HOME/ppg/escalations.jsonl`) |

The block persists for the same violation set; a submission hitting a
*different* set resets to the normal `422` path, and a successful lock
clears the streak. This detects the livelock **symptom** — general
unsatisfiability of a Rego corpus is undecidable and is not claimed. The
escalation log is the capitalization loop: each record is a conflict a
human must resolve, and the resolution belongs back in the corpus so the
same conflict cannot recur.

## `POST /register_skill`

Client-uploaded, session-scoped skill companion. The MCP server calls this
before every `/lock_in_plan` for every skill it finds under the project's
`.claude/skills/`; it is idempotent by content hash. Enables enforcement of
a locally-installed `SKILL.rego` against a gateway that does **not** share
the client's filesystem — the target scenario for a shared / remote gateway.
See [policy views](policy-views.md) for how the operator (`-skills`) and
session-scoped tiers compose.

Request:

| Field | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | ✅ | Isolates the registration; the same name in a different session is a separate entry |
| `name` | string | ✅ | Skill id — must match the `skill_id` a plan will declare |
| `skill_md` | string | ❌ | The `SKILL.md` body (stored for auditability; not currently evaluated) |
| `skill_rego` | string | ❌ | The `SKILL.rego` source. Omit for a tier-0 skill (no rego, no-op evaluator) |

Responses:

| Status | Body |
|---|---|
| `200` | `{status: SKILL_REGISTERED, session_id, name, has_rego}` |
| `422` | `{status: SKILL_COMPILE_ERROR, error, guidance}` — malformed Rego; nothing was stored, the prior registration under this name (if any) still applies |
| `400` | `{error}` — malformed body, or missing `session_id` / `name` |

Precedence at evaluation time: entries loaded by the operator via
`ppg -skills` **win over** any client-uploaded registration under the same
name. This prevents a project-local upload from silently downgrading an
org-wide policy the operator has already reserved.

### Lifetime & post-restart recovery

Session-scoped registrations live only in memory
(`internal/linter/linter.go` — `sessionSkills`). A gateway restart drops
every session-scoped skill; the operator tier is re-read from `-skills` but
client-uploaded skills are gone.

The MCP server self-heals this: it inspects every `/lock_in_plan` response
for `unknown_skill` violations naming a local skill and, when it finds
one, drops that entry from its content-hash cache, re-uploads via
`/register_skill`, and retries the lock exactly once. Bounded at one
retry — a second `unknown_skill` means the skill is genuinely missing
locally, so the semantic error reaches the model. See
`lockWithRegistrationRetry` in
[adapters/claudecode/mcpserver/main.go](../../adapters/claudecode/mcpserver/main.go).

Cross-session sharing is deliberately absent: a skill uploaded under
`session_id: "A"` is invisible to `session_id: "B"`. To distribute a
skill to every session on a shared gateway, load it via `ppg -skills`
(operator tier).

### Authentication & multi-user posture

Requests to `/register_skill` carry `session_id` verbatim from the client;
the gateway does not authenticate the caller. Same posture as the JWT
ticket signing key today (see the
[symmetric-key note](../explanation/design-decisions-and-limits.md#known-limits-of-the-poc)).

Two structural mitigations bound the blast radius of that trust:

- **Operator wins on name collision** — a client cannot downgrade an
  organisation-wide policy by re-uploading a permissive version under the
  same name.
- **Session-scoped isolation** — a rogue upload targeting session_id `A`
  only affects evaluations whose ticket carries `A`. Practical isolation
  relies on `session_id` being unguessable; `ppg-guard` `SessionStart`
  generates cryptographic UUIDs, which is sufficient for a trusted-team
  deployment on a private network.

An **enterprise multi-tenant** deployment should layer mTLS (or an
equivalent client-authentication scheme) in front of `/register_skill`
and bind each accepted `session_id` to a client identity, plus signed
skill manifests so a skill's provenance can be verified independently of
its uploader. Out of scope for the PoC.

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
[front matter](adr-front-matter.md) (`enforcement.altitudes`). Skill
companion `SKILL.rego` policies loaded via `ppg -skills` follow the same
model — when the ticket declared a `skill_id`, that skill's rules union
with the ADR corpus at every view. See
[policy views](policy-views.md) for the full input schemas.

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

## `POST /discover_service`

Capability discovery over the [service catalog](service-catalog.md): retrieves
the candidate services for a capability (or intent), ranks them with the
policy-as-code ranker, and returns the recommended one plus alternatives. Full
schema and examples in [service-catalog.md](service-catalog.md).

| Field | Type | Required | Description |
|---|---|---|---|
| `capability` | string | ❌\* | The capability needed (e.g. `notification`, `payment`) |
| `intent` | string | ❌\* | Natural-language intent; resolves the capability from selectors when `capability` is absent (\*supply one of the two) |
| `repository_context` | object | ❌ | Passed to the ranking policy for org rules (region, compliance, …) |

Response (`200`): `{status, capability, recommended, alternatives[],
policy_notes[]}`. `status` is `SERVICE_FOUND` when a service is recommended,
else `NO_SERVICE_FOR_CAPABILITY`. `503 SERVICE_CATALOG_UNAVAILABLE` when the
gateway was started without a catalog and/or ranking policy.

## `GET /services` · `GET /services/{id}`

`GET /services` returns `{services[]}` (the whole catalog). `GET /services/{id}`
returns one record, or `404 SERVICE_NOT_FOUND`.

## `GET /debt_report`

Response (`200`): `transition_debt_ratio`, `pending_sunsets[]`, `health`
(`OK` \| `DEBT_ALERT` when the ratio is ≥ 0.3).

## `POST /validate_skill`

Request and response are specified in [skill governance](skill-governance.md).
