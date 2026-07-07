# Explanation — understanding the why

> Discursive section. No code to run, no steps to follow: we explain the
> design decisions and their context.

## The underlying problem: from vibe coding to engineering at scale

Today's agentic interaction is **asymmetric and front-loaded**: all the
control is concentrated in the initial prompt and global context rules, then
the agent is on its own. The existing safety nets react **after the fact**
(broken CI, rejected commit), frustrating the flow instead of accelerating
it. This gateway inverts the paradigm by distributing governance *inside*
the agentic loop, starting with the first link after the intent:
**planning**.

## The founding distinction: two orthogonal axes

The intuitive mistake is to confuse *"does it block or guide?"* with
*"is it compensatory or amplifying?"*. They are different axes:

- **Durability axis** (compensatory ↔ amplifier): answers the *2× test*:
  "will this be more useful, or useless, when the model is twice as
  intelligent?"
- **Implementation axis** (declarative ↔ programmatic): fragile text vs
  deterministic code.

Locking a plan (hard gating) is neither inherently compensatory nor
amplifying: it depends on *what* the gate checks. Verifying a durable
semantic invariant ("a migration precedes the code") is amplifying;
exhaustively enumerating frozen files is compensatory; that is why the
policy is tagged and carries a sunset condition.

> 📖 Conceptual reference: the blog article *"See, Act, Correct: three levers
> for working with a code agent"* and its durability × implementation matrix.

## What `enrich()` actually does — and does not do

`enrich()` answers a single question, asked by the agent **before** it
plans: *"here is what I am about to do, and where: which of our
architectural decisions apply?"* The mechanics, step by step:

1. The agent sends its intent and repository context
   (`POST /enrich {intent, repository_context}`).
2. The gateway matches the intent against the **scope selectors** each ADR
   declares in its front matter (the intent "Add the Seka payment method"
   contains `payment` → ADR-042 matches). Keyword matching in the PoC,
   embedding-based retrieval in production; the contract is the same.
3. The matching invariants come back as the *amplifier context*; the agent
   injects them into its planning context and reasons over them. Its plan
   now honors the invariants before a single line is written.

Think of it as the fifteen-minute chat with the staff architect before
starting a piece of work: automated, exhaustive, and scoped to the task.
Or, in retrieval terms: RAG over the architecture knowledge base, keyed by
the intent.

Two deliberate non-goals define its boundary:

- **It advises, it never enforces.** Nothing stops an agent from ignoring
  the invariants at this stage. Enforcement is the plan linter's job, at
  `lock_in_plan` time. Same ADRs, two different moments: `enrich()` tells
  you the rules before you plan, the linter checks that your plan followed
  them.
- **It returns invariants, never recipes.** "Every external call goes
  through the egress proxy" — yes. "Modify `router.go` line 42" — never.

## Why `enrich()` contains zero hard-coded pattern

A first draft matched `if "payment" in intent` and appended recipes. Run
through the 2× test: **useless** once the model infers the patterns on its
own: declarative compensatory debt. The corrected version retrieves
**abstract semantic invariants** from the ADR store and lets the model
reason. An invariant like *"every external provider goes through the
security proxy"* stays true, and is better exploited, as models improve.
This is the declarative amplifier core of the solution.

## Why the capability ticket is NOT a brake

The ticket addresses the **symmetric risk**: an ungoverned amplifier also
amplifies systemic errors. If an agent (even a perfect one) applies a bad
practice at scale, the damage is proportional to its power. Least privilege
(the ticket only unlocks the planned scope) therefore stays relevant **even
against a perfect model**: it protects the organization, not the model.
That is why it is classified as an *amplifying* guardrail.

## Why verify the ticket inside the tool, not only upstream

Agentic drift happens **during** execution: the agent may call an unplanned
tool halfway through. The in-tool verification is the last deterministic
line of defense; the refusal (`OUT_OF_PLAN_SCOPE`) happens *before*
anything is executed: zero damage, zero cleanup.

## Why separate the generic translator from the semantic enrichers

To isolate the **compensatory debt** (raw→JSON translation, doomed to
sunset) from the **durable asset** (business-value feedback). The day models
read raw stack traces natively, the first is deleted without touching the
second. What remains durable is the context the model *cannot guess*: the
staging schema version, the interface definition, the violated ADR.

## Why plain Go policies

The conversation that designed this PoC used Open Policy Agent (OPA/Rego).
The rules here map one-to-one to Rego policies; plain Go keeps the PoC free
of external binaries so `go test ./...` runs anywhere. Production path:
OPA server or embedded engine, same registry, same tags.

## Why tag every rule and measure the debt

A durable platform must know **what it will have to remove**. Scaffolding is
useful at the start and becomes a hindrance if never dismantled. By tagging
each artifact (`amplifier` / `compensatory`) and forcing a measurable
`sunset_condition` on the compensatory ones, the platform team makes the
transition debt **visible** (report, ratio), gains an explicit exit
condition at every model upgrade, and avoids cementing obsolete crutches
into the organization. The compensatory ratio must **trend to zero**; this
PoC intentionally ships in `DEBT_ALERT` (2 scaffolding artifacts out of 5)
to make the mechanism visible.

## Alternatives considered and rejected

| Option | Why rejected |
|---|---|
| Everything in the system prompt | Fragile, non-deterministic, unverifiable: declarative compensatory. |
| Gate only in CI (post-hoc) | Purely after-the-fact: frustrates the flow, feedback arrives too late for self-correction. |
| An LLM to validate the plan | Non-deterministic → does not solve weak-model drift. We want a *linter*, not a judge. |
| Hard-coding business patterns in the gateway | Compensatory debt; does not scale; gains nothing from model progress. |

## Team Topologies positioning

The gateway is a **platform product** (internal SaaS) built by the Platform
team to reduce the cognitive load of stream-aligned teams and their agents.
ADRs are co-owned with the architects; programmatic policies belong to the
platform team; stream teams consume everything friction-free via MCP or the
pre-flight adapter. X-as-a-Service applied to agentic governance.

## Known limits of the PoC

- Keyword-based invariant retrieval (production: embeddings + reranking).
- Symmetric hard-coded JWT secret (production: KMS, asymmetric keys).
- Simulated sandbox (in-memory parse / staging state): no real workspace.
- Hard gating is not guaranteed on black-box tools (Copilot/Cursor).
- No persistence or telemetry yet: that is the third pillar (*observation*),
  intentionally out of scope here.
