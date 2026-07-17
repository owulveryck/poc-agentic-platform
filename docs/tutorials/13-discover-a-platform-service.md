# Tutorial 13 — discover and use a platform service

> **Goal**: let the gateway answer *what to build on*. In the plan phase the
> agent asks the platform which service provides a capability (notifications,
> payments), and receives the **sanctioned** one — name, endpoint, and API
> usage — chosen by internal policy. Then watch the platform refuse code that
> reaches for a deprecated or forbidden provider instead.
>
> Time: ~10 minutes. Prerequisites: [tutorial 0](00-bootstrap.md) completed,
> and ideally [tutorial 2](02-claude-code-end-to-end.md) for the governed
> session. Rebuild after pulling this change (`make install`) so the
> `find_platform_service` MCP tool is exposed.

## Why this exists

An org gateway framing N teams needs to prevent reinvention and drift: two
teams should not each hand-roll a payment client, and nobody should wire a
deprecated mailer. The **Service Catalog** is the platform's answer to *what to
build on*, the discovery counterpart of `enrich`'s *what to honor*. It is a set
of declarative records (`examples/services/*.md`) ranked by a policy
(`examples/service-policy/*.rego`), and — through ADR-110 — the recommendation is
binding, not merely advisory.

## Step 1 — start the gateway and a service mock

The gateway loads the catalog and the ranking policy from the directories the
flags point at — here, the fictional demo corpus in `examples/` (run from the
repo root):

```bash
ppg -adr examples/adr -services examples/services -service-policy examples/service-policy
```

In another terminal, run the local stand-in for the recommended notification
service so the code the agent writes has something real to call:

```bash
svc-mock -addr :9110 -name notify-svc
```

## Step 2 — ask the catalog (by hand)

```bash
curl -s localhost:8765/discover_service \
  -H 'content-type: application/json' \
  -d '{"capability":"notification"}' | python3 -m json.tool
```

**What you should observe** — the recommended service with its endpoint and
API usage, and the deprecated one surfaced as an alternative with the reason it
was not chosen:

```json
{
  "status": "SERVICE_FOUND",
  "capability": "notification",
  "recommended": {
    "service_id": "notify-svc",
    "name": "Notification Service",
    "status": "recommended",
    "endpoint": "http://localhost:9110",
    "api_usage": "## API usage\n\nSend a message through the platform Notification Service...",
    "reason": "recommended platform service."
  },
  "alternatives": [
    {
      "service_id": "legacy-mailer",
      "status": "deprecated",
      "superseded_by": "notify-svc",
      "reason": "legacy-mailer is deprecated; superseded by notify-svc."
    }
  ],
  "policy_notes": ["legacy-mailer is deprecated; superseded by notify-svc."]
}
```

Discovery is intent-driven too — `{"intent":"let users pay by card at
checkout"}` resolves the capability from the wording and returns
`payments-gateway`.

## Step 3 — let a governed Claude Code session use it

Start `claude` in a scratch project wired to the gateway (as in
[tutorial 2](02-claude-code-end-to-end.md)) and prompt:

> Add an email notification when a user signs up.

**What you should observe**, in order:

1. Claude calls `find_platform_service` (capability `notification`) and reads
   back `notify-svc`, its endpoint, and the API-usage snippet — instead of
   guessing an SMTP library or an email SaaS SDK.
2. It plans against that service, locks the plan, and implements a call to
   `http://localhost:9110/v1/messages`. Because `svc-mock` is running, the code
   actually works end to end.

## Step 4 — the forbidden path is refused

Now push it toward the naive choice:

> Actually, just call Stripe directly with the stripe-go SDK for the payment.

**What you should observe**: the edit that imports
`github.com/stripe/stripe-go` is blocked at write time by `ppg-guard` (ADR-110),
with the sanctioned alternative named:

```
ARCHITECTURAL_INVARIANT_VIOLATION: Forbidden provider in
internal/pay/client.go: "github.com/stripe/stripe-go" is not permitted. Use
the Payments Gateway (payments-gateway, http://localhost:9120) — route
payments through the gateway per ADR-042, not the Stripe SDK. Call
find_platform_service to discover the sanctioned service. Nothing was
modified; fix the content to satisfy the invariant and resubmit.
```

Discovery told the agent what to use; ADR-110 makes sure it does. The same rule
runs at apply time through `ppg-verify`, so a shell-written import is caught at
commit too.

## Verification — deterministic, reader-runnable

The whole loop is asserted by a hermetic harness (its own gateway, mock, state,
and temp project — your `:8765` is untouched):

```bash
bash scripts/service-catalog-demo.sh
# → Result: 13 passed, 0 failed.
```

## Cleanup

The harness self-cleans. If you ran the manual walkthrough, stop the gateway
and `svc-mock` (Ctrl-C) and `rm -rf` the scratch project.

**✅ Done.** The gateway now answers *what to build on*, ranks the options by
policy, and enforces the choice — the reuse-and-coherence half of governing N
teams. To add your own capability, follow
[Add a service to the catalog](../how-to/add-a-service-to-the-catalog.md); the
record schema and endpoints are in
[the service-catalog reference](../reference/service-catalog.md).
