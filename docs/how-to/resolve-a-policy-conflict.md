# Resolve a policy conflict (close the escalation loop)

**Goal**: an agent hit `409 POLICY_CONFLICT` — two rules are mutually
unsatisfiable for an intent, or the required plan shape is unreachable.
You are the human the escalation was addressed to. This page is the
capitalization half of the loop: diagnose the clash, fix the corpus,
close the conflict so it never recurs.

**When this happens**: the validation server escalates after 3 rejections
of a byte-identical violation set (consecutive or not). The block is
deliberate and sticky: it applies to **every** session that produces that
violation set, it survives server restarts, and no agent behavior can
clear it. Only the procedure below does.

## 1. List the open conflicts

```bash
ppg escalations list
```

```
CONFLICT      STATUS  FIRST SEEN            REJECTIONS  POLICY IDS                    DETAIL
2851de47b27c  OPEN    2026-07-21T13:25:46Z  4           go_tests_present              session 5555…
```

Every open conflict is one wall agents keep hitting. `POLICY IDS` names
the clashing rules; the `409` response carried the same `conflict_id`.

## 2. Inspect one conflict

```bash
ppg escalations show 2851de47b27c
```

Each record carries the intent, the declared `skill_id`, the plan steps,
the violation messages, and `policy_sources` — whether each policy comes
from the ADR corpus (`adr`), a skill companion (`skill`), or a built-in
rule (`built-in`). That is the room you need: the owners of those
corpora.

## 3. Fix the corpus (the actual resolution)

Decide which side is wrong and edit it — this is a policy decision, not a
CLI step:

- an ADR rule too broad → amend the `.rego` next to the ADR (and the ADR
  prose);
- a skill companion clashing with an ADR → fix the skill's `SKILL.rego`
  (and re-publish through Gate 1, `POST /validate_skill`);
- the intent itself was illegitimate → nothing to fix; the block **is**
  the correct outcome, leave the conflict open or resolve it with a note
  saying so.

Keep the fix deterministic and additive: the point of the loop is that
the same conflict can never happen again.

## 4. Close the conflict and reload

```bash
ppg escalations resolve 2851de47b27c -note "ADR-060: exempted docs-only intents"
kill -HUP "$(pgrep -x ppg)"
```

`resolve` removes the block from the livelock state and appends a
`resolution` record (with your note) to the escalation log — the audit
trail of every conflict a human closed. The SIGHUP makes the running
server adopt both the corpus fix and the resolution in one step. Until
the reload, the server's in-memory copy keeps answering
`POLICY_CONFLICT` — fail-closed, never fail-open.

## 5. Verify

```bash
ppg escalations list            # the conflict is gone
ppg escalations list -all       # …and recorded as RESOLVED with your note
```

Resubmit the previously blocked plan shape: it now gets an honest verdict
from the fixed corpus — `PLAN_LOCKED` if the fix legalized it, an
ordinary `422 PLAN_REJECTED` if it is still (correctly) refused. The
counter for that violation set starts from zero; if the "fix" did not
actually resolve the clash, the same wall escalates again after 3
rejections — with a fresh log trail pointing back at you.

## What this does NOT detect

The escalation fires on the livelock *symptom*: the same complete
violation set, three times, no successful lock in between. An agent whose
every submission produces a genuinely different violation set livelocks
undetected — general unsatisfiability of a Rego corpus is undecidable and
not claimed. See
[design decisions and limits](../explanation/design-decisions-and-limits.md).
