# Capability tickets and in-tool guards

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

For Claude Code, the same verification runs client-side as a `PreToolUse`
hook (`ppg-guard`): the hook exits with code 2 and the semantic message on
stderr goes back to the model, which self-corrects. For black-box agents
(Copilot, Cursor) no hook exists; only the soft half applies (pre-flight
instructions), and the locked-plan check must move to apply time — the
honest limit stated in the
[Copilot tutorial](../tutorials/03-github-copilot-preflight.md).

## Why separate the generic translator from the semantic enrichers

To isolate the **compensatory debt** (raw→JSON translation, doomed to
sunset) from the **durable asset** (business-value feedback). The day models
read raw stack traces natively, the first is deleted without touching the
second. What remains durable is the context the model *cannot guess*: the
staging schema version, the interface definition, the violated ADR.
