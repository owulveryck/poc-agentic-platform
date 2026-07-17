# How to add a new architectural invariant (ADR)

> Solves one problem: making a new architectural decision visible to every
> agent at planning time. Assumes you have completed the
> [tutorial](../tutorials/01-first-planning-cycle.md). First time authoring
> an ADR? Do the guided
> [write-your-first-ADR tutorial](../tutorials/05-write-your-first-adr.md)
> once, then come back here for the checklist.

1. Create `examples/adr/ADR-0XX-my-invariant.md` with the YAML front matter described
   in [adr-front-matter.md](../reference/adr-front-matter.md).
2. Apply the **classification test**: *"more useful, or useless, when the
   model is twice as intelligent?"*
   - More useful → `nature: amplifier`, `sunset_condition: null`.
   - Useless → `nature: compensatory` + a **mandatory, measurable**
     `sunset_condition`.
3. Fill `scope_selectors` with the keywords that make the invariant relevant.
4. State the invariant **semantically**: never "modify file X at line Y".
5. If the invariant has a deterministic check to express, pair it with a
   `.rego` policy: see
   [write-a-rego-plan-policy.md](write-a-rego-plan-policy.md). A
   declarative-only ADR (no `enforcement.rego`) is legitimate when the
   semantic directive alone is sufficient — ADR-042 is the example.
6. Verify: `curl -X POST /enrich` with an intent containing a selector; the
   invariant must be returned.
