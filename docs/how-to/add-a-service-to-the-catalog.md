# How to add a service to the catalog

> Solves one problem: making a shared capability discoverable (and, when
> needed, enforceable) so N teams reuse it instead of reinventing or drifting.
> Assumes you have run [tutorial 13](../tutorials/13-discover-a-platform-service.md).
> Record schema and endpoints:
> [service-catalog reference](../reference/service-catalog.md).

## Add a recommended (or allowed / sandbox) service

1. Create `examples/services/<service-id>.md` with YAML front matter + a body that is the
   **API usage** (how to call it — the body is returned to the agent verbatim):

   ```markdown
   ---
   service_id: audit-log
   name: Audit Log Service
   capability: audit
   status: recommended        # recommended | allowed | sandbox | deprecated | forbidden
   tier: 1                    # lower = higher priority (ranking tie-break)
   endpoint: http://localhost:9130
   owner_team: platform-observability
   selectors: ["audit", "audit log", "trail", "compliance log"]
   policy_tags: { region: eu }
   ---

   ## API usage

   POST http://localhost:9130/v1/events with {actor, action, target}. …
   ```

2. Pick `selectors` carefully. Retrieval is **case-insensitive substring** on
   the capability and the selectors (same as the ADR store), so `"db"` would
   match "add" — prefer distinctive multi-word terms.
3. Restart the gateway (it loads `examples/services/*.md` at startup). Confirm the log
   line `Service catalog loaded: N services`.
4. Verify:

   ```bash
   curl -s localhost:8765/discover_service -H 'content-type: application/json' \
     -d '{"capability":"audit"}' | python3 -m json.tool
   curl -s localhost:8765/services/audit-log | python3 -m json.tool
   ```

## Retire a service (deprecate) or ban one (forbid)

- **Deprecate**: set `status: deprecated` and `superseded_by: <new-id>` on the
  old record. Discovery keeps returning it as an *alternative* with the reason,
  never as the recommendation.
- **Forbid**: set `status: forbidden`. It is never recommended, and appears with
  a policy reason.

For a **forbidden or deprecated** provider you also want *enforced* (blocked in
written code), add its identifying marker to ADR-110 so the guard/`ppg-verify`
refuse it. This is the one place with two sources of truth kept in sync by hand:

1. Add the marker + guidance to `forbidden_markers` in `examples/adr/ADR-110.rego`.
2. Copy the file byte-for-byte to `internal/linter/testdata/ADR-110.rego`
   (the testdata mirror is required; see [AUDIT.md](../../AUDIT.md)).
3. Add a case to `internal/linter/linter_test.go`
   (`TestADR110ArtifactRejectsForbiddenProvider`).
4. `go test ./internal/linter/` and restart the gateway.

## Change the ranking policy

"Which is the best one" is policy-as-code in `examples/service-policy/*.rego`
(`package ppg.catalog`, rule `verdict`). To add an org rule — e.g. prefer a
service whose `policy_tags.region` matches the repo's region, or down-rank a
high-cost one — edit the `decide`/`score_for` rules there, mirror the file into
`internal/catalog/testdata/`, and add a `ranker_test.go` case. No Go changes.

## Rollback

Delete the `examples/services/<id>.md` file (and any ADR-110 marker you added, plus its
testdata mirror), restart the gateway. Discovery stops returning the service.

## What you lose (be honest)

- Retrieval is keyword/substring, not semantic — a capability phrased with none
  of the selectors will not match (PoC limit; production: embeddings).
- ADR-110 enforcement is substring-based content matching: a forbidden marker
  inside a comment or string literal is still flagged (regex is not a parser),
  and the marker list is maintained by hand alongside the catalog record.
