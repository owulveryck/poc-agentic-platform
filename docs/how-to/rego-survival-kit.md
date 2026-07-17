# Rego survival kit

> Solves one problem: writing your first plan or skill policy **without prior
> Rego knowledge**. This is not a Rego course; it is the 5-minute subset this
> platform actually uses. Every snippet below was validated against a running
> gateway. For the full language, see the
> [OPA docs](https://www.openpolicyagent.org/docs/latest/policy-language/).

## The mental model (read this once)

1. A `.rego` file is **not a program**. It is a set of rules; OPA evaluates
   all of them, in no particular order. No loops, no mutation, no early
   return.
2. `input` is the JSON document being judged (the plan at `lock_in_plan`,
   the skill at `validate_skill`). You never change it; you only derive
   facts from it.
3. You need exactly **one production pattern**:

   ```rego
   violation contains v if {
       # conditions (implicitly AND-ed, one per line)
       v := {"policy_id": "...", "message": "...", "nature": "..."}
   }
   ```

   Read it as: *the set of violations contains `v` whenever all the
   conditions hold.* No condition holds → no violation → the plan passes.
4. Iteration is implicit and **existential**: `some step in input.steps`
   means "there exists a step such that the following lines hold". If several
   bindings match, the rule fires once per match (several violations).
5. There is no `or` inside a rule. OR is spelled as **two rules with the
   same head** (see recipe 4).

Every file starts with the same two lines (the package puts your rules in
the linter's query; the import enables the `if`/`contains` keywords):

```rego
package ppg.linter        # ppg.skills.governance for skill rules

import rego.v1
```

## Recipe 1 — Require that a step exists

"A Go plan must contain a test step" (this is the real `ADR-060.rego`):

```rego
violation contains v if {
    input.repository_context.tech_stack[_] == "Go"   # only for Go stacks
    not plan_has_go_test                             # and no test step exists
    v := {
        "policy_id": "go_tests_present",
        "message":   "SDLC invariant violated: the plan has no test step. Add a step whose tool is \"go-test\", or whose action runs 'go test'.",
        "nature":    "amplifier",
    }
}

plan_has_go_test if {
    input.steps[_].tool == "go-test"
}

# Agents encode steps with their own tool names ("Bash" + a "go test"
# action): accept that too. Two rules with the same head = OR (recipe 4).
plan_has_go_test if {
    some step in input.steps
    contains(lower(step.action), "go test")
}
```

Two shapes to remember here. **"X must exist" is written as `not helper`
plus a helper rule that finds X** (do not try to negate an iteration inline;
see trap 1). And **write the message so it contains the machine-checkable
criterion**: an agent that reads "add a step whose tool is go-test" fixes
its plan in one iteration; an agent that reads "a test step must exist"
guesses.

## Recipe 2 — Forbid targets (deny-list)

"No step may touch a frozen path" (the real `ADR-070.rego`):

```rego
frozen_paths := {"internal/old_payment.go", "internal/auth/"}

violation contains v if {
    some step in input.steps
    some target in step.targets
    some fp in frozen_paths
    startswith(target, fp)
    v := {
        "policy_id": "explicit_frozen_files_enumeration",
        "message":   concat("", ["Frozen zone: modifying '", target, "' is forbidden (deprecated legacy code)."]),
        "nature":    "compensatory",
    }
}
```

Three `some` lines = a triple nested loop, for free. Each matching target
produces its **own** violation with its own message.

## Recipe 3 — Put the offending value in the message

`concat` works (above); `sprintf` is often more readable:

```rego
"message": sprintf("Database change '%s' without a migration step.", [t])
```

An actionable message is the whole point: the agent reads it and
self-corrects. Write the message for the agent, not for a logger.

## Recipe 4 — OR: two rules with the same head

"A target is database-related if it ends in `.sql` **or** lives under
`db/`" (the real `ADR-051.rego` uses exactly this):

```rego
target_is_db(step) if {
    endswith(step.targets[_], ".sql")
}

target_is_db(step) if {
    contains(step.targets[_], "db/")
}
```

Both rules define the same helper; OPA ORs them automatically. The same
mechanism is why every `.rego` file in the directory contributes to the same
`violation` set: rules with the same head union across files.

## Recipe 5 — "X must accompany Y" (companion step)

"Any database change requires a migration step in the same plan"
(`ADR-051.rego`, combining recipes 1 and 4):

```rego
violation contains v if {
    some step in input.steps
    target_is_db(step)              # a db change exists (recipe 4)
    not plan_has_migration          # but no migration step (recipe 1 shape)
    v := {
        "policy_id": "db_migration_precedes_code",
        "message":   "Invalid ordering: a schema migration step (tool 'db-migration-generator') must accompany any database change.",
        "nature":    "amplifier",
    }
}

plan_has_migration if {
    input.steps[_].tool == "db-migration-generator"
}
```

## The two traps that bite everyone

**Trap 1 — inline negation.** This does not mean "no step is a test step",
and the gateway will refuse to start:

```rego
not input.steps[_].tool == "go-test"
# → rego_unsafe_var_error: var _ is unsafe
```

`not` cannot wrap an expression that still needs to iterate. The fix is
always the same: name the existence in a helper rule, negate the helper
(recipe 1). If you take one thing from this page, take this.

**Trap 2 — assignment vs comparison.** `:=` declares a local (use it for
`v`), `==` compares (use it in conditions). Plain `=` unifies and is best
avoided until you know why you want it.

## Test your rule before shipping it

The fastest loop needs nothing but the gateway: point it at your ADR
directory and submit a plan that should fail.

```bash
ppg -addr :8765 -adr examples/adr     # watch: "Plan linter ready: N policies"
curl -s -X POST localhost:8765/lock_in_plan -H "Content-Type: application/json" -d @bad_plan.json
```

(`ppg` comes from `make install` — see the [top-level README](../../README.md#quick-start).)

A malformed rule fails fast: the gateway refuses to start and prints the
Rego error with file and line. A well-formed rule that never fires is the
case to guard against: always test one plan that **violates** and one that
**passes**, like the paired tests in `internal/linter/linter_test.go`
(copy your policy into `internal/linter/testdata/` and follow
[write-a-rego-plan-policy.md](write-a-rego-plan-policy.md), step 5).

If you have the [`opa` CLI](https://www.openpolicyagent.org/docs/latest/#running-opa)
installed, you can also evaluate a policy directly, without the gateway:

```bash
opa eval -d ADR-0XX.rego -i plan.json 'data.ppg.linter.violation'
```

## What you do NOT need

Everything else. No `default`, no `with`, no partial evaluation, no
functions beyond simple helpers, no data documents. The five production
policies of this repository (`examples/adr/*.rego`) and the two skill governance
files (`skill-governance/*.rego`) are all written with only the material on
this page; read them as your reference corpus.
