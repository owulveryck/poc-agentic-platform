# Capability tickets and in-tool guards

## Why the capability ticket is NOT a brake

The ticket addresses the **symmetric risk**: an ungoverned amplifier also
amplifies systemic errors. If an agent (even a perfect one) applies a bad
practice at scale, the damage is proportional to its power. Least privilege
(the ticket only unlocks the planned scope) therefore stays relevant **even
against a perfect model**: it protects the organization, not the model.
That is why it is classified as an *amplifying* guardrail.

## Why the ticket is bound to the session

A signed ticket with a TTL is still a **bearer capability**: whoever holds
the file holds the right, and a new session opened within the ticket's
wall-clock window would inherit it. The binding closes that window with the only
identifier both sides can observe: the `SessionStart` hook materializes the
real session id on disk, the MCP server stamps it into the plan at lock
time, and the guard compares it to the session id of every subsequent hook
invocation. The least-privilege scope was always the main containment (a
leaked ticket only ever opened the planned files); the binding turns the
remaining window into a non-event.

## Why verify the ticket inside the tool, not only upstream

Agentic drift happens **during** execution: the agent may call an unplanned
tool halfway through, or write bytes that satisfy the file scope yet break an
invariant. The in-tool verification is the last deterministic line of defense;
the refusal happens *before* anything is executed: zero damage, zero cleanup.
The guard checks two things on every edit: **which file** (path scope, against
the ticket — `OUT_OF_PLAN_SCOPE`) and **what content** (the artifact view of the
policy corpus — `ARCHITECTURAL_INVARIANT_VIOLATION`). For the content half it
POSTs the proposed bytes to the validation server's `/verify_artifact`, which runs the
same Rego rules that the plan linter and the apply-time gate use. The guard
**fails closed**: if it cannot evaluate an edit (unreadable payload, unopenable
store, unreachable validation server), it blocks with `PPG_GUARD_ERROR` rather than
letting the edit through.

For Claude Code, this runs client-side as a `PreToolUse` hook (`ppg-guard`):
the hook exits with code 2 and the semantic message on stderr goes back to the
model, which self-corrects. Copilot desktop and VS Code Copilot Chat get the
equivalent hard half through `ppg-copilot-guard` (a `PreToolUse` hook that emits
Copilot's JSON deny decision). The two guards behave identically — same
write-tool set, same content check.

For surfaces with no hook API (the `gh copilot` CLI, Cursor, a human at the
terminal, CI) the in-loop half is unavailable, so the check moves to **apply
time**: `ppg-verify` gathers the working-tree diff and asks the validation server's
`/verify_changeset` to evaluate the same corpus over the whole changeset —
exit 1 on a rejection, exit 2 (fail-closed) when it cannot run. Wired as a
pre-commit / pre-push hook or a CI step, it is the deterministic backstop for
the soft-only surfaces named in the
[Copilot tutorial](../tutorials/03-github-copilot-preflight.md). See the how-to
[Gate changes at apply time](../how-to/gate-changes-at-apply-time.md).

## Why separate the generic translator from the semantic enrichers

To isolate the **compensatory debt** (raw→JSON translation, doomed to
sunset) from the **durable asset** (business-value feedback). The day models
read raw stack traces natively, the first is deleted without touching the
second. What remains durable is the context the model *cannot guess*: the
staging schema version, the interface definition, the violated ADR.
