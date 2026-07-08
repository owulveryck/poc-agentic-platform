# From vibe coding to governed loops

> Discursive section. No code to run, no steps to follow: we explain the
> design decisions and their context.

## The underlying problem

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
policy is tagged and carries a sunset condition — see
[transition-debt.md](transition-debt.md).

> 📖 Conceptual reference: the blog article *"See, Act, Correct: three levers
> for working with a code agent"* and its durability × implementation matrix.
