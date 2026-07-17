---
service_id: payments-gateway
name: Payments Gateway
capability: payment
status: recommended
tier: 1
endpoint: http://localhost:9120
owner_team: platform-payments
selectors: ["payment", "pay", "charge", "checkout", "billing", "card"]
alternative_to: ["stripe-direct"]
policy_tags:
  region: eu
  pci: scoped
---

## API usage

Charge a customer through the platform Payments Gateway. The gateway is the
sanctioned egress for all payment providers (Stripe, Adyen, …) and satisfies
ADR-042 (every external provider call goes through the security egress proxy).
Do not call a provider SDK or a provider API directly.

```
POST http://localhost:9120/v1/charges
Content-Type: application/json

{
  "amount": 4200,               // minor units
  "currency": "eur",
  "customer_id": "cus_42",
  "provider": "stripe"          // routed by the gateway, not called directly
}
```

Response `201 Created`:

```json
{ "id": "chg_xyz789", "status": "authorized", "provider": "stripe" }
```

The gateway injects provider credentials server-side; your service never holds
a provider secret.
