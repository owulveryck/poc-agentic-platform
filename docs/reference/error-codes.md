# Statuses and error codes

| Code | Meaning |
|---|---|
| `CONTEXT_ENRICHED` | Enrichment succeeded |
| `PLAN_MALFORMED` | Structural contract violated (400) |
| `PLAN_REJECTED` | Policy violations, `violations[]` provided (422) |
| `PLAN_LOCKED` | Plan valid, ticket issued |
| `TOOL_NOT_IN_PLAN` | Tool absent from the ticket scope (403) |
| `OUT_OF_PLAN_SCOPE` | Target outside the allowed files (403) |
| `EXECUTION_FAILED` | Application failure; see `error_category` |
| `GO_SYNTAX_ERROR` | Patched content does not parse; guidance provided |
| `DATABASE_SCHEMA_CONFLICT` | Schema conflict; staging state provided |
| `SKILL_VALID` | Skill passes all governance policies; `tier` provided |
| `SKILL_REJECTED` | Governance violations, `violations[]` provided (422) |
| `linter_eval_error` | The OPA evaluation failed or its result was undecodable; the plan is rejected (fail closed) |
| `health: DEBT_ALERT` | Compensatory ratio ≥ 0.3 |
