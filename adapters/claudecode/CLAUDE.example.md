# Platform contract (governance harness)

- Before planning any change, call `get_platform_guidelines_for_intent` with
  the intent and the repository context: the platform returns the
  architectural invariants (ADRs) your plan must honor.
- Before modifying anything, submit your structured plan through
  `lock_in_plan`. No edit is accepted without a locked plan: a `PreToolUse`
  hook verifies every file against the capability ticket and blocks anything
  outside the locked scope.
- If a tool refuses with `OUT_OF_PLAN_SCOPE`, do not retry the same call:
  either stay within the locked plan, or re-plan through `lock_in_plan` if
  the extra change is genuinely needed.
- In the plan you lock, make platform-relevant steps explicit: a test step
  whose action runs `go test`, a migration step targeting `migrations/`.
  Violation messages name the exact criterion; fix precisely that.
