---
name: add-payment-method
description: Adds a payment provider to the checkout service through the governed loop of the Platform Planning Gateway, enriching the plan with the platform ADRs, locking it for a capability ticket, and implementing strictly within the ticket scope.
version: 2.0.0
argument-hint: "<provider name, e.g. Stripe>"
---

Add the payment provider named in $ARGUMENTS to the checkout service,
through the Platform Planning Gateway. Follow the three moves in order.

1. Call get_platform_guidelines_for_intent with the intent
   ("Add $ARGUMENTS as a payment method to the checkout service") and the
   repository context. Read every returned invariant before planning:
   they shape the content of your steps (egress proxy for external calls,
   frozen paths, migration ordering, tests).

2. Draft the structured plan honoring those invariants and submit it
   through lock_in_plan. If the gateway rejects it, the violation message
   names the exact criterion: fix precisely that and resubmit. On
   PLAN_LOCKED, the capability ticket is stored for the session.

3. Implement with Edit, staying strictly within the ticket scope. If the
   ppg-guard hook refuses with OUT_OF_PLAN_SCOPE, do not retry the same
   call: either stay within the locked plan, or re-plan through
   lock_in_plan if the extra change is genuinely needed.
