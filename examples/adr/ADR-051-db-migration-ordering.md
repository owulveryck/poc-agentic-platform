---
adr_id: ADR-051
title: Schema migrations precede code changes
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["database", "db", "schema", "migration", "sql", "table"]
enforcement:
  mode: programmatic
  policy_id: db_migration_precedes_code
  rego: ADR-051.rego
---

## Invariant

Any change touching persistent data MUST be accompanied by a schema migration
generated with the platform migration tool (`db-migration-generator`), and the
migration MUST precede the code that depends on it in the execution plan.

## Rationale (durability)

Ordering between schema and code is a property of the system, not of the
model. It remains true — and is checked deterministically by the plan linter —
regardless of how capable the implementer is.
