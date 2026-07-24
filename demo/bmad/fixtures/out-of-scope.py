"""An edit the Dev agent should NOT be making while implementing story 1.2.

The story's scope is the checkout service. Touching the auth module is
scope-creep — the classic "while I was in there…" drift. The ticket derived from
the story-scoped plan does not cover this path, so the guard refuses it with
OUT_OF_PLAN_SCOPE. Nothing about the file's *content* is wrong; it is simply not
this story's business.
"""


def rotate_session_secret() -> None:
    # Unrelated to the checkout story.
    ...
