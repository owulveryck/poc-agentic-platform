---
adr_id: ADR-140
title: The wide-scope cap stays a structural Go rule, not a Rego policy
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["scope", "ticket", "linter", "rego", "scope_breadth_cap"]
enforcement:
  mode: structural
  policy_id: scope_breadth_cap
---

## Context

The project's stated posture is "OPA/Rego is the only supported validation
format": every domain and architectural rule lives in the policy corpus so
that operators extend governance by writing Rego, never by patching Go.

Three checks nevertheless live in Go and fire before or beside the corpus:
plan structural validation (schema, step DAG, cycle detection), capability
ticket verification (signature, TTL, session binding, path-scope
matching), and the **wide-scope cap** (`scope_breadth_cap`,
`internal/linter/linter.go`): a plan whose `targets` include `.`/`*` — a
derived ticket that would be allow-all — is rejected at lock time unless
the server runs with `-allow-wide-scope`.

The first two are uncontroversially structural (contract and
cryptography). The wide-scope cap looks like a product rule, so the
2026-07-21 audit asked for an explicit decision: port it to Rego, or
record why it is structural.

## Decision

The wide-scope cap **stays in Go**. It is not a domain rule about what a
plan may do; it is a guard on the *ticket-derivation mechanism itself*
(`ticket.DeriveScope`): an allow-all scope would make every downstream
control point — the guards' path check, the Smart Tools' `AllowModify`
match, `ppg-verify` — vacuously true. A rule whose job is to keep the
enforcement machinery meaningful must not be hosted *inside* the machinery
it protects:

- **It must survive an empty or broken corpus.** The server runs with zero
  ADRs and zero skills (`-adr` omitted); the cap still applies. As Rego it
  would vanish exactly when the corpus is thinnest.
- **It must not be silently overridable by corpus content.** Operator- and
  session-provided Rego share the evaluation plumbing; the cap's toggle is
  a server *flag* (`-allow-wide-scope`) — a deliberate operator decision on
  the command line, visible in the process list, not a policy file a skill
  upload could shadow.
- **It is tier-independent and product-invariant.** Unlike domain rules,
  no organization is expected to customize *how* breadth is measured —
  only whether the cap is on (the flag).

The boundary this ADR fixes: **Go owns the checks that keep tickets and
plans meaningful (contract, crypto, scope breadth); Rego owns every rule
about what a meaningful plan/artifact may contain.** Anything new that
does not guard the mechanism itself goes in the corpus.

## Consequences

- `scope_breadth_cap` keeps its policy id so it surfaces in violations,
  `policy_sources` (as `built-in`), and conflict escalations like any
  other rule — the *reporting* is uniform even though the *hosting* is
  not.
- The built-in rules are enumerated in one place
  (`builtinPolicyIDs`, `cmd/ppg/conflict.go`) and documented in
  [policy views](../reference/policy-views.md); a new built-in requires
  amending this ADR's boundary test.
- Rejected alternative — a `builtin.rego` shipped inside the binary: it
  would keep the "Rego-only" slogan literally true but make the cap
  reload-fragile (SIGHUP swaps the corpus) and shadowable by name
  collision, for no operator benefit.
