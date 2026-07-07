---
adr_id: ADR-042
title: Every external provider call goes through the security proxy
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["payment", "paiement", "external", "third-party", "provider"]
enforcement:
  mode: declarative
  policy_id: external_call_via_proxy
---

## Invariant

Every outbound call to a third-party service (payment, KYC, notification)
MUST go through the corporate security egress proxy (`security-egress-proxy`).

## Rationale (durability)

This invariant stays true whatever the intelligence of the model: it is an
organizational security constraint, not a workaround for an LLM limitation.
A stronger model will honor it more elegantly — hence its AMPLIFIER nature.

## What we do NOT write here

We do not write "modify file X at line Y". We state the architectural intent
and let the model reason about the implementation.
