# Transition debt

## Why tag every rule and measure the debt

A durable platform must know **what it will have to remove**. Scaffolding is
useful at the start and becomes a hindrance if never dismantled. By tagging
each artifact (`amplifier` / `compensatory`) and forcing a measurable
`sunset_condition` on the compensatory ones, the platform team makes the
transition debt **visible** (report, ratio), gains an explicit exit
condition at every model upgrade, and avoids cementing obsolete crutches
into the organization. The compensatory ratio must **trend to zero**. This
PoC ships with 2 compensatory artifacts out of 7 total (ratio ≈ 0.29, just
under the 0.3 `DEBT_ALERT` threshold, so the report reads `health: "OK"`);
adding the two amplifier ADRs (ADR-090, ADR-100) diluted the ratio below the
alert line. Removing either amplifier — or adding a third compensatory
artifact — flips it back to `DEBT_ALERT`, which is exactly the signal the
report exists to surface.

The same model applies to skills: a skill that encodes a workaround for a
current model limitation is compensatory and should carry its sunset
condition; a skill that encodes a durable organizational workflow is an
amplifier. The PoC does not yet enforce sunset conditions on skills — see
[capability-plane-governance.md](capability-plane-governance.md) and
`AUDIT.md`.
