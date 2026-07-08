# Policy catalog

## Plan linter policies (`internal/linter.Registry`)

The registry is populated at startup from the ADR front matter
(`enforcement.policy_id`); the paired `.rego` files (package `ppg.linter`)
are compiled into a single OPA query over `data.ppg.linter.violation`.

| `policy_id` | Source ADR | Rego file | nature | sunset condition |
|---|---|---|---|---|
| `external_call_via_proxy` | ADR-042 | — (declarative-only) | amplifier | — |
| `db_migration_precedes_code` | ADR-051 | `adr/ADR-051.rego` | amplifier | — |
| `go_tests_present` | ADR-060 | `adr/ADR-060.rego` | amplifier | — |
| `explicit_frozen_files_enumeration` | ADR-070 | `adr/ADR-070.rego` | compensatory | model honors `@deprecated` >95% |

The debt report additionally counts one tagged translator:
`generic_raw_to_json_translator` (compensatory, sunset: model reads raw stack
traces reliably >95%). That makes 2 compensatory artifacts out of 5 — the
`DEBT_ALERT` this PoC intentionally ships with.

## Smart Tools (`internal/smarttools.Catalog`)

| Tool ID | Payload | Behavior |
|---|---|---|
| `patch_code` | `{"content": "<full patched Go file>"}` | Parses the patched content with `go/parser` in memory (simulated sandbox). Returns `GO_SYNTAX_ERROR` + `remediation_guidance` on parse failure |
| `apply_db_migration` | `{"statement": "CREATE TABLE ..."}` | Applies against a simulated staging schema (`payments`, `users` at `v2.4.1`). Returns `DATABASE_SCHEMA_CONFLICT` + staging context when the table exists |

Both verify the capability ticket before acting (`TOOL_NOT_IN_PLAN`,
`OUT_OF_PLAN_SCOPE`).

## Skill governance policies

Listed separately in [skill-governance.md](skill-governance.md).
