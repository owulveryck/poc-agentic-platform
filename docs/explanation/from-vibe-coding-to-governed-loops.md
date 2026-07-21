# From vibe coding to governed loops

> Discursive section. No code to run, no steps to follow: we explain the
> design decisions and their context.

## The underlying problem

Today's agentic interaction is **asymmetric and front-loaded**: all the
control is concentrated in the initial prompt and global context rules, then
the agent is on its own. The existing safety nets react **after the fact**
(broken CI, rejected commit), frustrating the flow instead of accelerating
it. This platform inverts the paradigm by distributing governance *inside*
the agentic loop, starting with the first link after the intent:
**planning**.

Underneath sits a harder fact: agents run on LLMs, and LLMs are
**non-deterministic**. Nothing guarantees that the rules present in the
context — instructions, skills, a design system — will be followed, and
the uncertainty gets worse when instructions conflict. If the design
system says buttons are blue and the developer types "No, I want them
pink", the outcome is pink *or* blue depending on the model's decisions
that day. Human-escalation safeguards don't close this: they themselves
depend on the model deciding to escalate.

## Why not an LLM judge?

The common mitigation is a second LLM reviewing the first one's output
("LLM-as-a-judge", sometimes marketed as adversarial validation). It
raises the bar, but structurally cannot provide a guarantee:

- **Cost** — every validation spends tokens, on every artifact, forever.
- **Insufficient trust** — the judge is non-deterministic too. It can let
  the same defect through twice; it *increases* confidence but cannot
  anchor it.
- **Not capitalizable** — when a judge misses something, nothing
  guarantees it won't miss it again. When a *deterministic* control point
  misses something, you extend the policy: that specific defect **never
  passes again**. The knowledge is capitalized in the system, not lost in
  a prompt.
- **Context fragility** — the more rules, directives and exceptions you
  load into the judge's context, the more it misses. Reliability degrades
  exactly as requirements accumulate — the opposite of what a guarantee
  system should do.

| Criterion | LLM-judge validation | Deterministic policy (this platform) |
|---|---|---|
| Determinism | No | Yes |
| Token cost | Per validation | Near zero |
| Trust | Partial, variable | High, reproducible |
| Fixing a defect | Not guaranteed to hold | Permanent (monotonic) |
| Sensitivity to context load | Degrades | Insensitive |
| Bypass possible | Yes (model-dependent) | No within the governed channel (blocking refusal) |
| Scope | Per prompt / per project | Machine level (organization) |

What determinism buys, concretely: the decision is **reproducible** (the
same plan or artifact gets the same verdict, whatever the model, however
long the session), **auditable** (the verdict cites the policy id and the
invariant behind it), and **free at the margin** (a Rego evaluation costs
microseconds, not tokens). And it is **monotonically improvable**: the
corpus only ever gets stronger, because every escaped defect becomes a
policy plus a regression test — see ADR-120's history in `AUDIT.md` for a
live example. Since v1.0 this determinism holds *by construction*, not by
convention: the OPA engine is compiled with every nondeterministic
built-in (`http.send`, `time.now_ns`, `rand.intn`, …) removed from its
capability set, so a policy that tries to depend on the network or the
clock fails to compile.

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
