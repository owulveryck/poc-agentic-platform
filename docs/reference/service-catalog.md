# Service catalog

The catalog is a declarative registry of the org's shared capabilities, loaded
at startup from the `-services` directory (`*.md` records) and ranked by the
`-service-policy` Rego — in this repo, the fictional demo corpus at
`examples/services/` and `examples/service-policy/`. It powers
capability discovery (`POST /discover_service`) and, together with ADR-110,
enforcement of the recommendation. See the
[tutorial](../tutorials/13-discover-a-platform-service.md) and the
[how-to](../how-to/add-a-service-to-the-catalog.md).

## Record schema (`examples/services/<id>.md`)

YAML front matter, then a Markdown body that is the **API usage** returned to
the agent.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `service_id` | string | yes | Unique id (e.g. `notify-svc`) |
| `name` | string | yes\* | Human-readable name (\*`service_id`+`capability` are the hard-required pair) |
| `capability` | string | yes | Category matched by discovery (e.g. `notification`, `payment`) |
| `status` | enum | yes | `recommended` \| `allowed` \| `sandbox` \| `deprecated` \| `forbidden` |
| `tier` | int | no | Ranking tie-break; lower = higher priority |
| `endpoint` | string | no | Base URL to reach the service |
| `owner_team` | string | no | Accountable team |
| `selectors` | string[] | no | Keywords for intent-driven discovery (case-insensitive substring) |
| `supersedes` | string[] | no | Service ids this one replaces (graph edge) |
| `superseded_by` | string | no | Successor id (set on `deprecated` records) |
| `alternative_to` | string[] | no | Sibling ids for the same capability |
| `policy_tags` | map | no | Free-form attributes the ranking policy may read (region, compliance, cost, …) |
| *(body)* | markdown | no | API usage; returned as `api_usage` |

## Ranking policy (`examples/service-policy/*.rego`, package `ppg.catalog`)

Query `data.ppg.catalog.verdict`. Input: `{capability, repository_context,
candidates: [<service>...]}`. Emits one verdict per candidate:

```json
{ "service_id": "notify-svc", "allow": true, "score": 99, "reason": "recommended platform service." }
```

The validation server sorts allowed candidates by `score` (then `tier`) to pick the
recommendation; denied candidates become alternatives. A candidate with no
verdict is treated as denied (fail-closed).

## Endpoints

### `POST /discover_service`

Request: `{ "capability"?: string, "intent"?: string, "repository_context"?: {…} }`
(supply `capability`, or `intent` to resolve it from selectors).

Response `200`:

```json
{
  "status": "SERVICE_FOUND",
  "capability": "notification",
  "recommended": { "service_id", "name", "capability", "status", "tier",
                   "endpoint", "owner_team", "api_usage", "reason" },
  "alternatives": [ { "service_id", "name", "status", "reason", "superseded_by"? } ],
  "policy_notes": [ "…" ]
}
```

`status` is `SERVICE_FOUND` when a recommended service exists, else
`NO_SERVICE_FOR_CAPABILITY` (no candidates, or all denied). `503`
`SERVICE_CATALOG_UNAVAILABLE` when the validation server has no catalog/ranking policy.

### `GET /services`

Returns `{ "services": [ <record>… ] }` (the whole catalog).

### `GET /services/{id}`

Returns the record, or `404` `SERVICE_NOT_FOUND`.

## MCP tool

`find_platform_service` (Claude Code / Copilot) forwards to `/discover_service`.
Args: `capability`, `intent`, `repository_name`, `tech_stack`. Call it in the
plan phase before integrating any shared capability.
