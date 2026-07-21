# Examples — a fictional demo corpus

Everything in this directory is **sample data for a fictional organization**.
The ADRs (ADR-042 … ADR-120), the service records (`notify-svc`,
`legacy-mailer`, `payments-gateway`, `stripe-direct`) and the ranking policy
are invented so the tutorials, `make quickstart`, and the demo scripts run
out of the box. None of it is product code: the validation server loads whatever
directories you point it at, and an adopting platform team replaces this
corpus with its own.

## Contents

| Directory | What it holds |
|---|---|
| `adr/` | 8 sample ADRs: YAML front matter + invariant prose, 7 of them paired with a `.rego` policy (`package ppg.linter`). ADR-042 is declarative-only. |
| `services/` | A sample service catalog: one `.md` record per shared service (front matter + verbatim API-usage body). Endpoints point at `svc-mock` or fake internal hosts. |
| `service-policy/` | The sample catalog ranking policy (`package ppg.catalog`, Rego). |

## Run the validation server against it

From the repository root:

```bash
ppg -addr :8765 -adr examples/adr \
    -services examples/services -service-policy examples/service-policy
```

`-adr` is required; `-services`/`-service-policy` are optional (omit them and
`/discover_service` is disabled).

## Replace it with your own corpus

Point the flags at your own directories. To author the content:

- ADR invariants: [docs/how-to/add-an-adr-invariant.md](../docs/how-to/add-an-adr-invariant.md),
  schema in [docs/reference/adr-front-matter.md](../docs/reference/adr-front-matter.md)
- Paired Rego policies: [docs/how-to/write-a-rego-plan-policy.md](../docs/how-to/write-a-rego-plan-policy.md)
- Service records: [docs/how-to/add-a-service-to-the-catalog.md](../docs/how-to/add-a-service-to-the-catalog.md),
  schema in [docs/reference/service-catalog.md](../docs/reference/service-catalog.md)

> Note: `skill-governance/` at the repository root is **not** example data —
> it is the product's default skill-publication policy and keeps its flag
> default.
