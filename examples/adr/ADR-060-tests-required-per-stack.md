---
adr_id: ADR-060
title: Every plan on a Go stack carries an executable test step
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["go", "golang", "test", "feature", "refactor"]
enforcement:
  mode: programmatic
  policy_id: go_tests_present
  rego: ADR-060.rego
---

## Invariant

Any execution plan targeting a Go codebase MUST contain a step that runs the
test suite (`go test`). The tests must exist and pass, whoever — human or
agent — writes the code.

## Rationale (durability)

The existence of an executable verification step is an SDLC invariant. A
smarter model writes better tests; it never makes the step useless.
