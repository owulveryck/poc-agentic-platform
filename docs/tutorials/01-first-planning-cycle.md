# Tutorial — your first amplified planning cycle

> **Goal**: run the full `enrich → plan → lock_in_plan → smart tool` cycle
> locally, and see with your own eyes a plan rejected, corrected, locked, and
> an out-of-scope action refused with zero damage.
>
> This tutorial does not try to explain everything: follow the steps in
> order; the *why* lives in the [explanation section](../explanation/).

## Step 0 — Prerequisites

- Go 1.25+
- `make install` from the repo root — puts `ppg` and the adapters under
  `~/.local/bin`.

## Step 1 — Start the gateway

From the repo root (`-adr` is required; `examples/` is the fictional demo
corpus):

```bash
ppg -addr :8765 -adr examples/adr \
    -services examples/services -service-policy examples/service-policy
```

You should see the readiness lines, then the listen line:

```
ADR store loaded: 8 invariants
Plan linter ready: 8 policies
Ticket signing key: ~/.local/state/ppg/ticket.key
Skill governance linter ready
Service catalog loaded: 4 services
Capability ticket TTL: 8h0m0s (bounded by the session)
Platform Planning Gateway listening on :8765
```

Keep this terminal open.

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
with `ADR-042` (security egress proxy) and `ADR-070` (frozen legacy paths).
👉 *The platform just injected semantic invariants without ever hard-coding
"if payment then…" anywhere.* Note also `compensatory_scaffolding`: the
platform is transparent about which parts of the injected context are
scheduled to disappear.

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
`go test` step, **including its `nature: amplifier`**:

```json
{
    "status": "PLAN_REJECTED",
    "violations": [
        {
            "policy_id": "go_tests_present",
            "message": "SDLC invariant violated: the plan has no test step. Add a step whose tool is \"go-test\", or whose action runs 'go test'.",
            "nature": "amplifier"
        }
    ],
    "guidance": "Fix the violations above and resubmit the plan."
}
```

👉 *This is semantic feedback: the platform does not say "no", it says "here
is what is missing".*

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

**What you should observe**: `"status": "PLAN_LOCKED"`, a `plan_hash`, and an
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
and the allowed list:

```json
{
    "status": "REFUSED",
    "code": "OUT_OF_PLAN_SCOPE",
    "attempted": "internal/auth/login.go",
    "allowed": [
        "migrations/001_seka.sql",
        "internal/payment/router.go",
        "tests/integration_payment_test.go"
    ],
    "guidance": "This action is not part of the locked plan. Re-plan through lock_in_plan if it is genuinely needed."
}
```

👉 *Nothing was executed. The last line of defense lives inside the tool,
exactly where agentic drift happens.*

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

**What you should observe**: `transition_debt_ratio` = `0.25`, two
`pending_sunsets`, and `health: OK` (this PoC ships with two of eight
artifacts as scaffolding — under the 0.3 `DEBT_ALERT` threshold). 👉
*You just measured how much temporary scaffolding the platform maintains,
and under which conditions it will be removed.*

**✅ Done.** You have run the complete cycle by hand. Next steps:

- [Tutorial 2 — govern a live Claude Code session](02-claude-code-end-to-end.md)
- [Tutorial 3 — steer GitHub Copilot with the pre-flight adapter](03-github-copilot-preflight.md)
- [Tutorial 4 — validate your first skill](04-validate-your-first-skill.md)
- To understand *why* it is designed this way, read the
  [explanation section](../explanation/).
