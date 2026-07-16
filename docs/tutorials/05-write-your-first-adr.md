# Tutorial — write your first ADR, end to end

> **Goal**: author a new architectural invariant (Markdown + Rego), load it
> into the gateway, and watch it steer an agent at `enrich` time and enforce
> at `lock_in_plan` time. You will govern the platform, not just use it.
>
> Time: ~10 minutes. Prerequisites: [tutorial 1](01-first-planning-cycle.md)
> completed. No Rego knowledge needed; when in doubt, open the
> [Rego survival kit](../how-to/rego-survival-kit.md).

The invariant we will encode: *every new HTTP endpoint must be registered in
the OpenAPI spec*. Realistic, checkable, and it exercises the full
dual-representation pattern.

## Step 1 — Work on a copy of the ADR store

```bash
cp -r adr /tmp/my-adr-store
```

The gateway takes the store as a flag, so you can experiment without
touching the repository corpus.

## Step 2 — Write the semantic half (Markdown)

Create `/tmp/my-adr-store/ADR-080-openapi-registration.md`:

```markdown
---
adr_id: ADR-080
title: Every new HTTP endpoint is registered in the OpenAPI spec
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: [endpoint, api, route, handler]
enforcement:
  mode: programmatic
  policy_id: openapi_registration_present
  rego: ADR-080.rego
---
## Invariant

Every new or modified HTTP endpoint MUST be reflected in `api/openapi.yaml`
in the same change set. The spec is the contract consumers rely on; code and
contract never diverge.
```

Two decisions happened here. The **classification**: run the 2× test ("more
useful, or useless, when the model is twice as intelligent?"); a contract
invariant stays true whatever the model, so `nature: amplifier` and no
sunset condition. The **retrieval**: `scope_selectors` are the words that
make this ADR relevant to an intent; pick the words a developer would
actually type.

## Step 3 — See the soft half work

Start the gateway on your copy and enrich an intent containing a selector:

```bash
ppg -addr :8769 -adr /tmp/my-adr-store
```

```bash
curl -s -X POST localhost:8769/enrich \
  -H "Content-Type: application/json" \
  -d '{"intent": "Add a status endpoint to the API",
       "repository_context": {"name": "checkout-service", "tech_stack": ["Go"]}}' \
  | python3 -m json.tool
```

**What you should observe**: `ADR-080` in `architectural_invariants`, with
your invariant text verbatim. The word "endpoint" in the intent matched your
selectors; any agent planning this task now reasons over your rule. Note
also the startup log: `ADR store loaded: 7 invariants` (the 6 shipped ADRs
plus your ADR-080).

## Step 4 — Write the executable half (Rego)

Create `/tmp/my-adr-store/ADR-080.rego` (this is
[recipe 5](../how-to/rego-survival-kit.md#recipe-5--x-must-accompany-y-companion-step)
of the survival kit: "X must accompany Y"):

```rego
package ppg.linter

import rego.v1

violation contains v if {
    some step in input.steps
    contains(step.targets[_], "handler")
    not spec_is_updated
    v := {
        "policy_id": "openapi_registration_present",
        "message":   "Contract drift: a handler changes but api/openapi.yaml is not in the plan. Add a step targeting api/openapi.yaml.",
        "nature":    "amplifier",
    }
}

spec_is_updated if {
    input.steps[_].targets[_] == "api/openapi.yaml"
}
```

Write the message so it contains the criterion (add a step targeting
`api/openapi.yaml`): the agent that receives it must be able to fix the plan
without guessing.

## Step 5 — See the hard half work

Restart the gateway (watch `Plan linter ready: 7 policies`), then submit a
plan that touches a handler without the spec:

```bash
curl -s -X POST localhost:8769/lock_in_plan \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "11111111-1111-1111-1111-111111111111",
    "intent": "Add a status endpoint to the API",
    "repository_context": {"name": "checkout-service", "tech_stack": ["Go"]},
    "steps": [
      {"id":"s1","action":"add status handler","tool":"Write","targets":["internal/api/status_handler.go"]},
      {"id":"s2","action":"go test ./...","tool":"go-test","targets":["internal/api/status_handler_test.go"]}
    ]
  }' | python3 -m json.tool
```

**What you should observe**: `422 PLAN_REJECTED` with your
`openapi_registration_present` violation and your message, verbatim. Add the
missing step and resubmit:

```json
{"id":"s2","action":"register the endpoint","tool":"Edit","targets":["api/openapi.yaml"]}
```

**What you should observe**: `PLAN_LOCKED`. Your rule now runs on every plan
the gateway sees, deterministically.

## Step 6 — Ship it

When the rule is ready for the real corpus: move the two files into `adr/`,
add a paired test (copy the policy into `internal/linter/testdata/` and
follow [write-a-rego-plan-policy.md](../how-to/write-a-rego-plan-policy.md),
step 5), and restart the gateway. If your invariant is compensatory instead
of amplifier, set a measurable `sunset_condition`: the
[debt report](01-first-planning-cycle.md#step-7--read-the-transition-debt-report)
will track it until you retire it.

**✅ Done.** You wrote a rule once as prose for the agent, once as policy for
the linter, and watched each half act at its own moment of the loop. The
*why* of this dual representation is in
[dual-representation-adr.md](../explanation/dual-representation-adr.md).
