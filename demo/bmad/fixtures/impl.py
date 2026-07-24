"""Checkout payment service — the in-scope implementation for story 1.2.

This is the file the story's Dev Notes name ("src/checkout/service.py"), so it is
inside the ticket scope derived from a plan that reads the story. The guard lets
it through.
"""

import os


def charge_saved_card(customer_id: str, amount_cents: int) -> dict:
    # Secret from the environment, not a literal; external call would go through
    # the egress proxy in the real service.
    api_key = os.environ["STRIPE_API_KEY"]
    try:
        return _stripe_payment_intent(api_key, customer_id, amount_cents)
    except StripeDeclined as exc:
        raise PaymentDeclined(str(exc)) from exc


class StripeDeclined(Exception):
    pass


class PaymentDeclined(Exception):
    pass


def _stripe_payment_intent(api_key: str, customer_id: str, amount_cents: int) -> dict:
    raise NotImplementedError
