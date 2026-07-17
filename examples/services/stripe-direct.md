---
service_id: stripe-direct
name: Stripe (direct SDK)
capability: payment
status: forbidden
tier: 9
endpoint: https://api.stripe.com
owner_team: none
selectors: ["stripe", "payment", "charge"]
alternative_to: ["payments-gateway"]
policy_tags:
  region: us
  pci: unscoped
---

## API usage (forbidden — routed through the gateway instead)

Calling Stripe directly (the `github.com/stripe/stripe-go` SDK or
`https://api.stripe.com`) is **forbidden**: it bypasses the security egress
proxy (ADR-042), spreads provider credentials into product services, and puts
each team in PCI scope. Use `payments-gateway`, which routes to Stripe
server-side. This record exists so discovery and ADR-110 can name what is
forbidden and why.
