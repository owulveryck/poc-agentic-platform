# Transition debt

## Why tag every rule and measure the debt

A durable platform must know **what it will have to remove**. Scaffolding is
useful at the start and becomes a hindrance if never dismantled. By tagging
each artifact (`amplifier` / `compensatory`) and forcing a measurable
`sunset_condition` on the compensatory ones, the platform team makes the
transition debt **visible** (report, ratio), gains an explicit exit
condition at every model upgrade, and avoids cementing obsolete crutches
into the organization. The compensatory ratio must **trend to zero**; this
PoC intentionally ships in `DEBT_ALERT` (2 scaffolding artifacts out of 5)
to make the mechanism visible.

The same model applies to skills: a skill that encodes a workaround for a
current model limitation is compensatory and should carry its sunset
condition; a skill that encodes a durable organizational workflow is an
amplifier. The PoC does not yet enforce sunset conditions on skills — see
[capability-plane-governance.md](capability-plane-governance.md) and
`AUDIT.md`.
