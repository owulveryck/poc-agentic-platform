# How to add a Smart Platform Tool

> Solves one problem: exposing a new execution capability that verifies the
> ticket in-tool and returns semantic feedback.

1. Create a package under `internal/smarttools/<name>/` implementing the
   `smarttools.Tool` interface (`ID`, `Run`).
2. Sandbox first: act on an isolated copy, never directly on the target.
3. Guard your inputs: `Run` receives `targets` and `payload` straight from
   the HTTP request body — validate emptiness and shapes before indexing.
4. In the analysis step, call `translate.Generic` for the raw outcome
   (compensatory) then a **semantic enricher** with the business context the
   model cannot guess (amplifier). Aim for an *actionable*
   `remediation_guidance`, not a descriptive one.
5. Register it in `cmd/ppg/main.go` with `smarttools.Register`.
6. The ticket guard is inherited: any out-of-scope call is refused before
   your code runs.
