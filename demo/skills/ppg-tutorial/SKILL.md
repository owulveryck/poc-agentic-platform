---
name: ppg-tutorial
description: Runs the full amplified planning loop of the validation server end-to-end against a local instance. Use to demonstrate, explore, or test the PPG live. Shows enrich, a deterministic plan rejection with semantic guidance, the capability ticket, an out-of-scope refusal, and the debt report — with the real transcript at every step.
version: 1.0.0
argument-hint: "[intent, e.g. Add Stripe as a payment method to the checkout service]"
---

Run the full amplified planning cycle of the validation server (PPG)
and narrate each step to the user with the actual JSON transcripts. Use Bash
for every command. Work from the poc-agentic-platform repository root; clone
https://github.com/owulveryck/poc-agentic-platform first if it is absent.

The intent for the whole session is $ARGUMENTS; if empty, use:
"Add Stripe as a payment method to the checkout service".

1. Validation server. If nothing listens on localhost:8765, start it in the
   background: `go run ./cmd/ppg -addr 127.0.0.1:8765 -adr examples/adr
   -services examples/services -service-policy examples/service-policy`,
   and wait for
   "validation server listening".

2. Enrich. POST the intent to /enrich with repository_context
   {"name": "checkout-service", "tech_stack": ["Go"]}. Show the returned
   architectural_invariants and point out that the ADRs were selected by
   the intent's keywords.

3. Rejection. POST to /lock_in_plan a plan with a single step editing
   internal/payment/router.go and no test step. Expect 422 PLAN_REJECTED
   with the go_tests_present violation; show the violation message and
   explain that it names the exact criterion to fix.

4. Lock. Resubmit with three steps: a migration targeting
   migrations/001_stripe.sql, the router edit, and a "go test ./..." step.
   Expect PLAN_LOCKED; save the execution_ticket and decode its claims
   (base64url payload) to show allow_modify and allow_tool.

5. Refusal. POST to /tools/patch_code with the ticket and the target
   internal/auth/login.go. Expect 403 OUT_OF_PLAN_SCOPE; show that the
   response lists the allowed files and that nothing was modified.

6. Debt. GET /debt_report and show transition_debt_ratio, pending_sunsets
   and health.

7. GitHub Copilot variant. Run
   `go run ./adapters/preflight -repo checkout-service -stack Go "<intent>"`
   and show the generated .github/copilot-instructions.md: the same
   invariants, served to a black-box agent as repository custom
   instructions.

Finish with a two-column summary: what the platform did at each step, and
where the same feedback would have arrived without it (CI or code review,
days later). Link the user to docs/tutorials/ for the deeper walkthroughs.
