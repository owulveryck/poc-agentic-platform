# Tutorial — from a governed skill to a governed session, end to end

> 👉 Prefer to *watch* it first? Open the 90-second animated tour of the whole
> chain (tutorials 01→06): [ppg-tutorials-tour.svg](../diagrams/ppg-tutorials-tour.svg).

> **Goal**: live the full lifecycle in one sitting. You play the stream
> team: write a skill and its companion policy, pass them through the
> platform's publication gate, then watch the skill drive a real Claude
> Code session through every gateway (enrich, lock, in-tool guard).
>
> Time: ~20 minutes. Prerequisites:
> [tutorial 2](02-claude-code-end-to-end.md) completed: the gateway runs on
> `:8765`, and `~/ppg-demo` has the `ppg-guard` hooks (`SessionStart` +
> `PreToolUse`), the `ppg` MCP server, and the `CLAUDE.md` contract.
>
> In a hurry? The finished skill ships in the repository's demo package
> (`apm install owulveryck/poc-agentic-platform/demo --target claude`).
> But writing it yourself is the point of this tutorial.

## Step 1 — You are the team: write the skill and its policy

A skill is how a team distributes a workflow to every developer's agent.
Create `.claude/skills/add-payment-method/SKILL.md` in `~/ppg-demo`:

```markdown
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
   repository context. Read every returned invariant before planning.

2. Draft the structured plan honoring those invariants and submit it
   through lock_in_plan. If the gateway rejects it, the violation message
   names the exact criterion: fix precisely that and resubmit.

3. Implement with Edit, staying strictly within the ticket scope. If the
   ppg-guard hook refuses with OUT_OF_PLAN_SCOPE, do not retry the same
   call: re-plan through lock_in_plan if the change is genuinely needed.
```

Notice what the body is: not a list of rules to remember, but a **workflow
that puts the gateway inside the loop**. The two tool names are the ones
the `ppg` MCP server registered in
[tutorial 2](02-claude-code-end-to-end.md): `get_platform_guidelines_for_intent`
bridges to `POST /enrich`, `lock_in_plan` to `POST /lock_in_plan`. Then
write the companion policy,
`.claude/skills/add-payment-method/SKILL.rego`:

```rego
package ppg.skills.add_payment_method

import rego.v1

violation contains v if {
    some step in input.steps
    endswith(step.targets[_], ".go")
    not plan_has_migration
    v := {
        "policy_id": "payment_provider_migration_first",
        "message":   "A payment provider needs its schema migration: add a step targeting a file under migrations/ before the code steps.",
        "nature":    "amplifier",
    }
}

plan_has_migration if {
    some step in input.steps
    startswith(step.targets[_], "migrations/")
}
```

The pair is a dual-representation artifact: `SKILL.md` is what the agent
executes; `SKILL.rego` is what the platform can verify. That is the
division of labor: **the team ships the capability and its policy**.

## Step 2 — The publication gate

Before a skill reaches anyone's agent, it passes the platform's gate.
Build the request from your two files and submit it (from
`~/ppg-demo`, gateway running on `:8765`):

```bash
python3 - <<'EOF' > /tmp/skill.json
import json
raw = open('.claude/skills/add-payment-method/SKILL.md').read()
fm, body = raw[4:].split('\n---\n', 1)
meta = dict(l.split(':', 1) for l in fm.strip().split('\n'))
meta = {k.strip(): v.strip().strip('"') for k, v in meta.items()}
print(json.dumps({
    "name": meta["name"], "description": meta["description"],
    "version": meta["version"], "argument_hint": meta["argument-hint"],
    "body": body.strip(),
    "rego_policy": open('.claude/skills/add-payment-method/SKILL.rego').read(),
}))
EOF
curl -s -X POST localhost:8765/validate_skill \
  -H "Content-Type: application/json" --data @/tmp/skill.json | python3 -m json.tool
```

**What you should observe**:

```json
{
    "status": "SKILL_VALID",
    "tier": 1
}
```

Tier 1: the body instructs `Edit`, so the companion policy was mandatory
(remove `rego_policy` from the payload to see the gate refuse). Curious
about rejections? Delete the `version` line and resubmit: `422
SKILL_REJECTED` with the exact field named. The full rejection tour is
[tutorial 4](04-validate-your-first-skill.md); in CI this exact call is the
publish gate ([how-to](../how-to/gate-skill-publication-in-ci.md)).

## Step 3 — The governed session

Start `claude` in `~/ppg-demo` and type the same command a developer would:

```
/add-payment-method Stripe
```

**What you should observe**, in order — every gateway of the platform,
triggered by the skill's own body:

1. The skill executes: its first instruction makes Claude call
   `get_platform_guidelines_for_intent`, the MCP bridge to the enrichment
   gateway. The intent contains "payment", so ADR-042 (every external
   provider call goes through the egress proxy) and ADR-070 (frozen legacy
   paths) come back as invariants: the same payload you saw in
   [tutorial 1, step 2](01-first-planning-cycle.md).
2. Claude drafts the plan **already honoring them** (migration first,
   proxied client, test step) and submits it through `lock_in_plan`. If a
   violation comes back, it reads the criterion and resubmits: one
   round-trip.
3. `PLAN_LOCKED`: the capability ticket is persisted through the
   per-machine TokenStore
   (`$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<sid>`), bound to this
   session id (recorded by the `SessionStart` hook in the SessionStore).
   A ticket reused from another session would be refused
   (`SESSION_MISMATCH`).
4. Implementation: every in-scope `Edit` passes the guard silently. Ask for
   *"also quickly update internal/auth/login.go"* to see the drift blocked
   before execution, exit code 2, nothing modified.

## Step 4 — The gates you just crossed

| Gate | Endpoint / mechanism | Moment | Who owns it |
|---|---|---|---|
| Publication | `POST /validate_skill` | Skill enters the registry | Platform (gate), team (skill + policy) |
| Soft guidance | `POST /enrich` | Before the plan | Platform (retrieval), architects (ADRs) |
| Hard lock | `POST /lock_in_plan` (OPA linter, ticket) | At plan submission | Platform (linter), teams (paired `.rego`) |
| In-tool guard | `ppg-guard` hook, `/tools/{name}` | At every action | Platform |

One sentence to keep: **the team writes the skill and its policy; the
platform exposes the gates, the schema, and the enforcement; the agent
executes the skill, and everything it does passes through the platform's
endpoints.**

**✅ Done.** For black-box agents that cannot call the gateways in-loop, the
soft half still applies: see the
[GitHub Copilot tutorial](03-github-copilot-preflight.md). The *why* of the
capability plane is in
[capability-plane-governance.md](../explanation/capability-plane-governance.md);
how an intent maps to ADRs (scope selectors, semantic retrieval) is in
[enrichment-and-planning.md](../explanation/enrichment-and-planning.md).
