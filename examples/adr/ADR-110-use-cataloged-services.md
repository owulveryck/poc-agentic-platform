---
adr_id: ADR-110
title: Integrate shared capabilities through the cataloged service
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["payment", "paiement", "charge", "checkout", "billing", "notification", "notify", "email", "sms", "stripe"]
enforcement:
  mode: programmatic
  policy_id: use_cataloged_services
  rego: ADR-110.rego
  altitudes: [plan, artifact, changeset]
---

## Invariant

When a change needs a shared capability (payment, notification, storage, …),
it MUST build on the service the platform catalog recommends for that
capability — discovered via `find_platform_service` (or `POST
/discover_service`) — and MUST NOT integrate a deprecated or forbidden
provider directly. Concretely, for the seed catalog:

- Payments go through the **Payments Gateway** (`payments-gateway`,
  `http://localhost:9120`), never the Stripe SDK
  (`github.com/stripe/stripe-go`) or `api.stripe.com` directly — this is the
  concrete "which service" for ADR-042's egress-proxy invariant.
- Notifications go through the **Notification Service** (`notify-svc`,
  `http://localhost:9110`), never the deprecated `legacy-mailer`.

## Rationale (durability)

Reuse and coherence across N teams are organizational properties, not
LLM-limitation workarounds: a smarter model still should not stand up a
second payment integration or wire a deprecated mailer. The catalog answers
*what to build on*; this ADR makes that answer binding. Hence AMPLIFIER — no
sunset.

## What we do NOT write here

We do not enumerate every service (they live in the catalog's `*.md` records,
canonical — `examples/services/` in this repo) or their ranking (that is the
`-service-policy` Rego). We state the intent — use the
cataloged service — and let discovery supply the endpoint and API usage.

## Enforcement stack

- Plan altitude (`input.view == "plan"`): a plan that names a forbidden
  provider directly (e.g. "integrate Stripe") is rejected with a pointer to
  the recommended service.
- Artifact / changeset altitudes: written code containing a forbidden
  provider marker (`github.com/stripe/stripe-go`, `api.stripe.com`, the
  `legacy-mailer.internal` host) is denied at write time (`ppg-guard` →
  `/verify_artifact`) and at apply time (`ppg-verify` → `/verify_changeset`).

> **Honest limit.** The forbidden markers are declared in two places — the
> catalog record (`status: forbidden|deprecated`) and this policy's
> `forbidden_markers` set — kept in sync by hand, like the skill-tier logic
> flagged in `AUDIT.md`. Adding a forbidden/deprecated service means updating
> both (see `docs/how-to/add-a-service-to-the-catalog.md`). And, as with every
> content rule, matching is substring-based: a marker inside a comment or
> string literal is still flagged (regex is not a parser).
