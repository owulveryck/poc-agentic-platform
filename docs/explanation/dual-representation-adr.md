# The dual-representation ADR

Each ADR with programmatic enforcement is a **dual-representation governance
artifact**: it carries two complementary encodings of the same architectural
decision, serving two distinct moments in the agentic loop.

| Representation | Field | Consumed by | Moment |
|---|---|---|---|
| Semantic directive | `InvariantText` (Markdown body) | `enrich()` → agent planning context | Before the plan |
| Rego policy | `Enforcement.RegoFile` (`.rego` file) | `lock_in_plan` linter (OPA evaluation) | At plan submission |

The **semantic directive** is injected into the agent's planning context at
`enrich()` time. It is prose the model reasons over: *"every external call
must go through the egress proxy"*. The agent uses it to shape the *content*
of each plan step.

The **Rego policy** is evaluated deterministically at `lock_in_plan` time by
the embedded [Open Policy Agent](https://www.openpolicyagent.org/) engine.
It does not reason; it checks. A conforming plan passes silently; a
non-conforming plan gets a structured `violation` object (with `policy_id`,
`message`, and `nature`) returned to the agent for self-correction.

The two representations are decoupled on the **durability axis**. A semantic
directive may be a permanent amplifier (the invariant is always useful) while
the paired Rego policy is compensatory scaffolding (it will be retired once
the model reliably honors the invariant without explicit enforcement). ADR-042
demonstrates the opposite case: a declarative-only ADR with no Rego file —
the semantic directive alone is sufficient because the invariant has no
deterministic check to express.

Although a single `.rego` file, the policy half is exercised at **three
altitudes** — plan, artifact, and changeset — discriminated by the
`input.view` field. One file, three moments in the loop: rejecting a bad
plan at `/lock_in_plan`, rejecting a bad edit at `/verify_artifact`
(called from the guard hook), and rejecting a bad diff at
`/verify_changeset` (the apply-time backstop). See
[policy views](../reference/policy-views.md) for the input schemas and
the guard idiom.

## Why OPA/Rego for plan enforcement

The plan linter uses the embedded [Open Policy Agent](https://www.openpolicyagent.org/)
Go library (`github.com/open-policy-agent/opa`). All `.rego` files in the
ADR directory are loaded at startup into a single `PreparedEvalQuery` over
`data.ppg.linter.violation`; their `violation contains v` rules union
automatically across files.

This choice keeps governance rules **outside the binary**: an architect edits
a `.rego` file alongside the ADR Markdown, and the linter picks it up at
next startup — no recompilation. The `violation` object schema (`policy_id`,
`message`, `nature`) maps directly to the `Violation` struct returned to the
agent, so the connection between the Rego rule and its agent-facing rejection
message is explicit and auditable.

The linter also **fails closed**: an OPA evaluation error, or a violation
result that does not decode into the expected shape, rejects the plan with a
`linter_eval_error` violation instead of silently passing it. A gate that
fails open is not a gate.

The same engine and the same failure posture govern skills — see
[capability-plane-governance.md](capability-plane-governance.md).
