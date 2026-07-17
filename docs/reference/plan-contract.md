# Plan contract

The plan is the structured JSON contract submitted to `POST /lock_in_plan`.
The language-neutral schema is [`schemas/plan.schema.json`](../../schemas/plan.schema.json);
the Go twin is [`internal/plan.Plan`](https://pkg.go.dev/github.com/owulveryck/poc-agentic-platform/internal/plan#Plan),
from which the MCP tool schema is auto-generated.

| Field | Type | Required | Description |
|---|---|---|---|
| `session_id` | uuid | ✅ | Session identifier |
| `intent` | string (≥5) | ✅ | Natural-language intent |
| `stream_aligned_team` | string | ❌ | Owning team; used for audit and routing |
| `skill_id` | string | ❌ | Published skill driving this plan. Gate 3: the linter unions the skill's companion Rego (`ppg -skills`); an unknown id rejects the plan (`unknown_skill`) |
| `repository_context.name` | string | ✅ | Repository name |
| `repository_context.tech_stack` | string[] | ✅ | Detected stacks |
| `repository_context.current_branch` | string | ❌ | Branch being worked on |
| `steps[].id` | string | ✅ | Step identifier |
| `steps[].action` | string | ✅ | Human-readable action |
| `steps[].tool` | string | ✅ | Platform tool invoked |
| `steps[].targets` | string[] | ✅ | Files/resources touched |
| `steps[].depends_on` | string[] | ❌ | Graph dependencies; each entry must name another step's `id`, and the graph must be acyclic (validated at `lock_in_plan`) |

Structural validation (`ValidateStructure`) rejects with `400 PLAN_MALFORMED`;
policy validation (the OPA linter) rejects with `422 PLAN_REJECTED`. See
[http-api.md](http-api.md).

`steps[].tool` is free-form. The linter policies recognize both the platform
vocabulary (`go-test`, `db-migration-generator`, `patch_code`) and the
natural encodings coding agents produce (a `Bash` step whose action runs
`go test`; a migration expressed as a step targeting `migrations/`).
