# How to write a deterministic plan policy (Rego)

> Solves one problem: turning an ADR into a rule the plan linter enforces at
> `lock_in_plan` time, 100% reproducibly. Policies are OPA/Rego files paired
> with ADRs — nothing is compiled into the binary.
>
> New to Rego? Read the [Rego survival kit](rego-survival-kit.md) first
> (5 minutes, covers everything this platform uses).

1. Create `examples/adr/ADR-0XX.rego` next to the ADR Markdown, in package
   `ppg.linter`, using the `violation contains v if {...}` idiom:

   ```rego
   package ppg.linter

   import rego.v1

   violation contains v if {
       some step in input.steps
       # ... your deterministic condition over the plan ...
       v := {
           "policy_id": "my_policy_id",
           "message":   "Actionable, agent-facing explanation of what to fix.",
           "nature":    "amplifier",
       }
   }
   ```

   The `input` document is the whole plan (see the
   [plan contract](../reference/plan-contract.md)). Violation rules from all
   `.rego` files union automatically.

2. Declare the pairing in the ADR front matter:

   ```yaml
   enforcement:
     mode: programmatic
     policy_id: my_policy_id
     rego: ADR-0XX.rego
   ```

   The `policy_id` registers the policy in the linter `Registry` (and thus in
   the debt report); the `rego` field tells the linter which file to load.

3. Tag the `nature` in both the ADR and the violation object. If
   `compensatory`, set a measurable `sunset_condition` in the front matter —
   the debt report tracks it.

4. Restart the gateway; the startup log must show the incremented policy
   count (`Plan linter ready: N policies`).

5. Add a test in `internal/linter/linter_test.go` with a copy of the policy
   in `internal/linter/testdata/` (mind the drift: testdata regos are copies
   of the `examples/adr/` ones), then run:

   ```bash
   go test ./internal/linter/ ./internal/debt/
   ```

   The debt test also confirms the compensatory ratio stays acceptable.
