# Tutorial — your first amplified planning cycle

> **Goal**: run the full `enrich → plan → lock_in_plan → smart tool` cycle
> locally, and see with your own eyes a plan rejected, corrected, locked, and
> an out-of-scope action refused with zero damage.
>
> This tutorial does not try to explain everything: follow the steps in
> order; the *why* lives in [explanation.md](explanation.md).

## Step 0 — Prerequisites

- Go 1.23+

## Step 1 — Start the gateway

```bash
go run ./cmd/ppg -addr :8765
```

You should see `ADR store loaded: 4 invariants` and
`Platform Planning Gateway listening on :8765`. Keep this terminal open.

## Step 2 — Enrich an intent (the amplifier phase)

```bash
curl -s -X POST localhost:8765/enrich \
  -H "Content-Type: application/json" \
  -d '{
    "intent": "Add the Seka payment method to checkout",
    "repository_context": {"name": "checkout-service", "tech_stack": ["Go"]}
  }' | python3 -m json.tool
```

**What you should observe**: a response containing `architectural_invariants`
with `ADR-042` (security egress proxy). 👉 *The platform just injected a
semantic invariant without ever hard-coding "if payment then…" anywhere.*
Note also `compensatory_scaffolding`: the platform is transparent about which
parts of the injected context are scheduled to disappear.

## Step 3 — Submit a deliberately incomplete plan (see the rejection)

```bash
curl -s -X POST localhost:8765/lock_in_plan \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "11111111-1111-1111-1111-111111111111",
    "intent": "Add Seka payment",
    "repository_context": {"name": "checkout-service", "tech_stack": ["Go"]},
    "steps": [
      {"id":"s1","action":"edit router","tool":"patch_code","targets":["internal/payment/router.go"]}
    ]
  }' | python3 -m json.tool
```

**What you should observe**: a `422` with a violation naming the missing
`go test` step, **including its `nature: amplifier`**. 👉 *This is semantic
feedback: the platform does not say "no", it says "here is what is missing".*

## Step 4 — Correct the plan (get the capability ticket)

```bash
curl -s -X POST localhost:8765/lock_in_plan \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "11111111-1111-1111-1111-111111111111",
    "intent": "Add Seka payment",
    "repository_context": {"name": "checkout-service", "tech_stack": ["Go"]},
    "steps": [
      {"id":"s1","action":"generate migration","tool":"db-migration-generator","targets":["migrations/001_seka.sql"]},
      {"id":"s2","action":"edit router","tool":"patch_code","targets":["internal/payment/router.go"]},
      {"id":"s3","action":"go test ./...","tool":"go-test","targets":["tests/integration_payment_test.go"]}
    ]
  }' | python3 -m json.tool
```

**What you should observe**: `"status": "PLAN_LOCKED"` and an
`execution_ticket` (a JWT string). 👉 *This is the signed contract without
which no execution tool will work.* Export it:

```bash
export TICKET="<paste the execution_ticket value>"
```

## Step 5 — Try to drift (see the deterministic refusal)

```bash
curl -s -X POST localhost:8765/tools/patch_code \
  -H "Content-Type: application/json" \
  -d "{\"ticket\":\"$TICKET\",\"targets\":[\"internal/auth/login.go\"],\"payload\":{\"content\":\"package auth\"}}" \
  | python3 -m json.tool
```

**What you should observe**: `OUT_OF_PLAN_SCOPE`, with the attempted target
and the allowed list. 👉 *Nothing was executed. The last line of defense
lives inside the tool, exactly where agentic drift happens.*

## Step 6 — Fail legitimately (see the semantic feedback)

```bash
curl -s -X POST localhost:8765/tools/patch_code \
  -H "Content-Type: application/json" \
  -d "{\"ticket\":\"$TICKET\",\"targets\":[\"internal/payment/router.go\"],\"payload\":{\"content\":\"package payment\nfunc Broken( {\"}}" \
  | python3 -m json.tool
```

**What you should observe**: `error_category: GO_SYNTAX_ERROR` with a
`remediation_guidance` block. 👉 *The tool is a deterministic mentor: the
agent knows what to fix, not just that it failed.* Resubmit with valid
content and you get `status: OK`.

## Step 7 — Read the transition-debt report

```bash
curl -s localhost:8765/debt_report | python3 -m json.tool
```

**What you should observe**: a `transition_debt_ratio` and the list of
`pending_sunsets` (this PoC intentionally ships with a `DEBT_ALERT`: two of
five artifacts are scaffolding). 👉 *You just measured how much temporary
scaffolding the platform maintains, and under which conditions it will be
removed.*

**✅ Done.** You have run the complete cycle. To understand *why* it is
designed this way, read [explanation.md](explanation.md).
