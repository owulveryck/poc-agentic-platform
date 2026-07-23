# Telemetry event reference

The `ppg.*` wide-event vocabulary of the decision journal
(`internal/journal`, constants in `internal/journal/names.go`). One decision
event is emitted **exactly once, by the component that made the decision**;
clients emit only client-side facts the server cannot see. This table is the
contract consumed by `ppg report`, the live dashboard, and a future OTLP
adapter (field mapping: `Time`→Timestamp, `Name`→EventName,
`Severity`→SeverityText, `Component`→`service.name`, `SessionID`→`session.id`,
`Attrs`→Attributes).

Severity is uniform across events: **INFO** = allowed/normal, **WARN** = a
guardrail correctly denied something, **ERROR** = infrastructure failure /
fail-closed.

## Events

| Event | Emitter | Severity | Attributes |
|---|---|---|---|
| `ppg.session.start` | guards | INFO | `agent` (`claude-code`\|`copilot`) |
| `ppg.intent.declared` | ppg-mcp-server | INFO | `intent`, `repo` — the loop-entry marker; carries the session id that the server's `/enrich` request lacks |
| `ppg.enrich.served` | ppg | INFO | `intent`, `repo`, `invariant_count` — carries the session id when the caller provides one in the request (the MCP server does); session-less otherwise |
| `ppg.service.discovered` | ppg | INFO / WARN when no service | `capability`, `status`, `service_id`, `alternatives_count` |
| `ppg.plan.malformed` | ppg | WARN | `reason` |
| `ppg.plan.rejected` | ppg | WARN | `policy_ids`, `violation_count`, `rejection_count`, `intent`, `skill_id` |
| `ppg.plan.conflict` | ppg | WARN | `conflict_id`, `policy_ids`, `rejections`, `intent`, `skill_id` |
| `ppg.plan.locked` | ppg | INFO | `plan_hash`, `step_count`, `target_count`, `intent`, `skill_id`, `ticket_ttl_s` |
| `ppg.plan.substitution` | ppg | WARN | `expected_hash`, `got_hash` |
| `ppg.skill.registered` / `ppg.skill.rejected` | ppg | INFO / WARN | `skill`, `has_rego` (registered only) |
| `ppg.artifact.rejected` | ppg | WARN | `path`, `op`, `policy_ids` — no OK counterpart (redundant with `ppg.guard.allow`) |
| `ppg.scope.refused` | ppg | WARN | `code`, `attempted` — from `/verify_artifact`, `/verify_changeset`, `/tools/{name}`; `session_id` may be empty when the ticket itself failed |
| `ppg.changeset.ok` / `ppg.changeset.rejected` | ppg | INFO / WARN | `file_count` (+ `policy_ids` on rejection) |
| `ppg.guard.allow` | guards | INFO | `tool`, `path` — one per gated write tool that passed every gate: the "act" counter |
| `ppg.guard.block` | guards | WARN; ERROR when fail-closed | `tool`, `path`, `reason_code` (below) — never the violation message body |
| `ppg.ticket.saved` | ppg-mcp-server | INFO | `plan_hash` — the ticket reached the store the guard reads |
| `ppg.lock.retry` | ppg-mcp-server | INFO | `unknown_skill_count` — self-heal retry; lock verdicts are never re-emitted client-side |
| `ppg.client.error` | ppg-mcp-server | ERROR | `route`, `kind` |
| `ppg.verify.run` | ppg-verify | INFO(`ok`) / WARN(`rejected`) / ERROR(`error`) | `mode` (`staged`\|`worktree`), `outcome`, `status`, `file_count` |

## `reason_code` values of `ppg.guard.block`

| Code | Meaning | Severity |
|---|---|---|
| `no_ticket` | write attempted before any plan was locked | WARN |
| `out_of_plan_scope` | target path outside the ticket's `allow_modify` | WARN |
| `session_mismatch` | ticket issued for another session | WARN |
| `ticket_rejected` | signature/TTL/shape verification failed | WARN |
| `invariant_violation` | content check (`/verify_artifact`) denied the edit | WARN |
| `guard_error` | the guard could not evaluate: fail-closed | ERROR |

## Double-counting rule

A guard block with `reason_code=invariant_violation` coexists with the
server's `ppg.artifact.rejected` for the same edit — different layers: the
guard owns `tool`/`path`/verdict, the server owns `policy_ids` (which never
cross the wire back to the guard). Consumers count **blocks** from
`ppg.guard.block` only and attribute **policy ids** from server events only.

## Payload attributes

When payload capture is enabled (default; kill switch
`PPG_TELEMETRY_PAYLOADS=off`), decision events add bounded (32 KiB per side)
payload attributes — the material behind the dashboard modal's request/reply
panes:

| Attribute | On | Contains |
|---|---|---|
| `request` | `ppg.plan.*` (submitted plan), `ppg.changeset.rejected` (paths + plan hash) | what the caller sent — never file contents |
| `response` | plan verdicts, `ppg.artifact.rejected`, `ppg.changeset.rejected`, `ppg.plan.substitution`, `ppg.scope.refused` | the JSON verdict returned (violations, guidance) — never the execution ticket |
| `reply` | `ppg.guard.block` | the model-facing block message |
| `request_omitted` / `response_omitted` | any of the above | size note when a payload exceeded the cap |

## Privacy contract

Events carry paths, hashes, policy ids, byte counts, and the plan intent —
never file contents, edit payloads, or the execution ticket. The plan-altitude
payloads above are the same exposure class as `escalations.jsonl`, which
already persists full plans and violations. Caveat: violation *messages* (in
`response`/`reply`) may quote governed content — disable payload capture
before shipping the journal off the machine.
