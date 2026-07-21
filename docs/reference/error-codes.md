# Statuses and error codes

| Code | Meaning |
|---|---|
| `CONTEXT_ENRICHED` | Enrichment succeeded |
| `PLAN_MALFORMED` | Structural contract violated (400) |
| `PLAN_REJECTED` | Policy violations, `violations[]` provided (422) |
| `PLAN_LOCKED` | Plan valid, ticket issued |
| `TOOL_NOT_IN_PLAN` | Tool absent from the ticket scope (403) |
| `OUT_OF_PLAN_SCOPE` | Target outside the allowed files (403) |
| `SESSION_MISMATCH` | Ticket's `session_id` claim does not match the active session; emitted by the guards to block a ticket replayed from another session |
| `SERVICE_FOUND` | `/discover_service` returned a recommended service for the capability |
| `NO_SERVICE_FOR_CAPABILITY` | `/discover_service` found no candidate (or all were denied) for the capability |
| `SERVICE_CATALOG_UNAVAILABLE` | `/discover_service` called on a validation server started without a catalog and/or ranking policy (503) |
| `SERVICE_NOT_FOUND` | `GET /services/{id}` for an unknown service id (404) |
| `ARTIFACT_OK` | `/verify_artifact` accepted the edited content (artifact view) |
| `ARTIFACT_REJECTED` | `/verify_artifact`: the file scope is allowed but the content breaks an invariant, `violations[]` provided (422) |
| `CHANGESET_OK` | `/verify_changeset` accepted the whole diff (changeset view) |
| `CHANGESET_REJECTED` | `/verify_changeset`: the diff breaks an invariant, `violations[]` provided (422) |
| `PLAN_SUBSTITUTION` | `/verify_changeset`: the supplied `plan_hash` does not match the ticket's claim; `expected`/`got` provided (409) |
| `POLICY_CONFLICT` | `/lock_in_plan`: 3 consecutive rejections with a byte-identical violation set — livelock escalation; `policy_ids`, `policy_sources` (adr\|skill\|built-in) and the `escalation_log` path provided; the block persists until the violation set changes (409) |
| `ARCHITECTURAL_INVARIANT_VIOLATION` | Content broke an architectural invariant; emitted by the guards (block reason) and Smart Tools (`error_category`) after the path scope passed |
| `PPG_GUARD_ERROR` | A guard could not evaluate an edit (unreadable payload, unopenable store, unreachable validation server) and blocked the PreToolUse edit fail-closed |
| `EXECUTION_FAILED` | Application failure; see `error_category` |
| `GO_SYNTAX_ERROR` | Patched content does not parse; guidance provided |
| `DATABASE_SCHEMA_CONFLICT` | Schema conflict; staging state provided |
| `SKILL_VALID` | Skill passes all governance policies; `tier` provided |
| `SKILL_REJECTED` | Governance violations, `violations[]` provided (422) |
| `linter_eval_error` | The OPA evaluation failed or its result was undecodable; the plan is rejected (fail closed) |
| `health: DEBT_ALERT` | Compensatory ratio ≥ 0.3 |
