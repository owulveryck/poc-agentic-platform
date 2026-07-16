# How to retire compensatory scaffolding (pay back the debt)

> Solves one problem: removing a crutch once the model no longer needs it.

1. Verify the `sunset_condition` is met (new model benchmark).
2. Remove the paired `.rego` file and the `enforcement.policy_id` /
   `enforcement.rego` entries from the ADR front matter (or remove the
   translator, for tool-side scaffolding).
3. If the underlying intent remains valid, **promote** it to an amplifier
   invariant instead of deleting it: keep the semantic directive, drop only
   the deterministic check.
4. Mark the source ADR `status: superseded` and document the retirement.
5. Re-run the test suite: the transition-debt ratio must have decreased.

   ```bash
   go test ./internal/debt/
   curl -s localhost:8765/debt_report
   ```
