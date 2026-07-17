---
adr_id: ADR-070
title: Frozen legacy files are enumerated explicitly
status: accepted
nature: compensatory
sunset_condition: "Model honors '@deprecated' annotations semantically on >95% of an internal benchmark."
scope_selectors: ["legacy", "payment", "refactor", "old"]
enforcement:
  mode: programmatic
  policy_id: explicit_frozen_files_enumeration
  rego: ADR-070.rego
---

## Invariant

The following paths are frozen and MUST NOT be modified by an agent:
`internal/old_payment.go`, `internal/auth/`.

## Rationale (durability)

This exhaustive enumeration is SCAFFOLDING: it compensates for the model's
current inability to infer "deprecated legacy code" from annotations alone.
When the sunset condition is met, this ADR is retired — or promoted to a
semantic invariant ("do not modify code marked @deprecated") if still useful.
