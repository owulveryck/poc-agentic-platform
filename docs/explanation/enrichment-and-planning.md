# Enrichment and planning

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

## How the agent knows what plan to submit

The question "how does the agent know what to put in the plan?" has three
separate answers, each at a different layer:

- **When** to call `lock_in_plan`: the behavioral rule lives in `CLAUDE.md`
  (or `CLAUDE.example.md` for this PoC). It tells the agent that no
  modification is accepted without a locked plan.
- **How** to format the plan: the MCP server exposes
  [`plan.Plan`](https://pkg.go.dev/github.com/owulveryck/poc-agentic-platform/internal/plan#Plan)
  as a typed Go struct; the SDK generates the JSON Schema automatically and
  delivers it to the agent at session startup. The agent never needs to be
  told the format in prose.
- **What** to put in the plan: the invariants returned by
  `get_platform_guidelines_for_intent` (i.e. `enrich()`) are injected into
  the agent's planning context. The agent reasons over them and shapes the
  content of each step accordingly.

| Layer | Source | Controls |
|---|---|---|
| **When** to call `lock_in_plan` | `CLAUDE.md` | Behavioral rule |
| **How** to format the plan | MCP tool schema (from `plan.Plan`) | JSON structure |
| **What** the plan must contain | `enrich()` invariants | Semantic content |

The schema validates structure deterministically; the linter validates ADR
compliance deterministically; the model fills in the business content from
the enriched context. None of the layers overlap.

## Why `enrich()` contains zero hard-coded pattern

A first draft matched `if "payment" in intent` and appended recipes. Run
through the 2× test: **useless** once the model infers the patterns on its
own: declarative compensatory debt. The corrected version retrieves
**abstract semantic invariants** from the ADR store and lets the model
reason. An invariant like *"every external provider goes through the
security proxy"* stays true, and is better exploited, as models improve.
This is the declarative amplifier core of the solution.
