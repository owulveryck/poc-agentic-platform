# Policy catalog

## Plan linter policies (`internal/linter.Registry`)

The registry is populated at startup from the ADR front matter
(`enforcement.policy_id`); the paired `.rego` files (package `ppg.linter`)
are compiled into a single OPA query over `data.ppg.linter.violation`.

| `policy_id` | Source ADR | Rego file | nature | altitudes | sunset condition |
|---|---|---|---|---|---|
| `external_call_via_proxy` | ADR-042 | — (declarative-only) | amplifier | — | — |
| `db_migration_precedes_code` | ADR-051 | `adr/ADR-051.rego` | amplifier | plan | — |
| `go_tests_present` | ADR-060 | `adr/ADR-060.rego` | amplifier | plan | — |
| `explicit_frozen_files_enumeration` | ADR-070 | `adr/ADR-070.rego` | compensatory | plan | model honors `@deprecated` >95% |
| `design_tokens_referenced` | ADR-090 | `adr/ADR-090.rego` | amplifier | plan, artifact | — |
| `per_machine_state_directory` | ADR-100 | `adr/ADR-100.rego` | amplifier | plan | — |

## Enforcement altitudes

The same corpus is evaluated at three altitudes, keyed by `input.view` (see
[Policy at three altitudes](http-api.md#policy-at-three-altitudes)):

- **`plan`** (lock time) — rules read `input.steps`; a violation rejects the
  plan at `/lock_in_plan`. Every programmatic policy implements this by default.
- **`artifact`** (in-loop) — rules read `input.artifact` (`{path, content,
  op}`); the guards and Smart Tools call `/verify_artifact` with a single edit's
  actual content. `ADR-090` uses this to reject raw hex/`rgb()`/`hsl()`/named
  colors (including `var(--x, #F0F)` fallbacks) and button re-styling
  (`button:hover`, `button > span`, `.btn`, `[role="button"]`) outside
  `design/tokens.css`.
- **`changeset`** (apply time) — rules read `input.changeset.files` (a list of
  `{path, content, op}`); `ppg-verify` and CI call `/verify_changeset` over the
  whole diff. `ADR-090`'s content rules cover this altitude too.

Each ADR declares the altitudes its `.rego` implements in its front matter
(`enforcement.altitudes`).

The debt report additionally counts one tagged translator:
`generic_raw_to_json_translator` (compensatory, sunset: model reads raw stack
traces reliably >95%). That makes 2 compensatory artifacts out of 7 total
(`transition_debt_ratio` ≈ 0.29), so the debt report reports `health: "OK"`.

## Smart Tools (`internal/smarttools.Catalog`)

| Tool ID | Payload | Behavior |
|---|---|---|
| `patch_code` | `{"content": "<full patched Go file>"}` | Parses the patched content with `go/parser` in memory (simulated sandbox). Returns `GO_SYNTAX_ERROR` + `remediation_guidance` on parse failure |
| `apply_db_migration` | `{"statement": "CREATE TABLE ..."}` | Applies against a simulated staging schema (`payments`, `users` at `v2.4.1`). Returns `DATABASE_SCHEMA_CONFLICT` + staging context when the table exists |

Both verify the capability ticket before acting (`TOOL_NOT_IN_PLAN`,
`OUT_OF_PLAN_SCOPE`), then evaluate the artifact-view policy over the payload
content (`content` or `statement`). Content that breaks an invariant returns
`EXECUTION_FAILED` with `error_category: ARCHITECTURAL_INVARIANT_VIOLATION` and
`remediation_guidance.violations[]`, without running the tool.

## Skill governance policies

Listed separately in [skill-governance.md](skill-governance.md).
