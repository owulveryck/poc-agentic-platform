# Story 1.2: Add Stripe as a checkout payment method

Status: ready-for-dev

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a returning customer,
I want to pay for my basket with a saved Stripe card,
so that I can check out without re-entering card details.

## Acceptance Criteria

1. A customer with a saved Stripe card can complete checkout in one step.
2. A declined charge returns a typed error and leaves the basket intact.
3. No card data is stored in our database; only the Stripe customer id is persisted.

## Tasks / Subtasks

- [ ] Task 1: charge service (AC: #1, #2)
  - [ ] Subtask 1.1: call Stripe PaymentIntents with the saved customer id
  - [ ] Subtask 1.2: map decline codes to typed errors
- [ ] Task 2: persistence (AC: #3)
  - [ ] Subtask 2.1: store only the Stripe customer id

## Dev Notes

- Relevant architecture patterns and constraints: all external provider calls go
  through the egress proxy; secrets come from the environment, never literals.
- Source tree components to touch: `src/checkout/service.py`.
- Testing standards summary: unit-test the decline-code mapping.

### Project Structure Notes

- Alignment with unified project structure: payment logic lives under
  `src/checkout/`.

### References

- Cite all technical details with source paths and sections, e.g.
  [Source: docs/architecture.md#payments]

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
