# Tutorial 9 — Copilot on a governed workstation

> **Goal**: prove that once the workstation is configured user-wide,
> spinning up a governed Copilot session on a brand-new project takes
> three shell commands and a single prompt. No `.github/hooks/`, no
> `.github/copilot-instructions.md`, no APM per-project — the whole
> ceremony from tutorial 7 disappears.
>
> Time: ~5 minutes.
> Prerequisites:
> - [Tutorial 0 — bootstrap](00-bootstrap.md) completed.
> - [How-to — set up a governed workstation](../how-to/set-up-a-governed-workstation.md)
>   applied for the Copilot recipe: user-scope MCP, hooks in
>   `~/.copilot/hooks/ppg.json`, contract in
>   `~/.copilot/copilot-instructions.md`, and optionally skills in
>   `~/.copilot/skills/`.

## Step 1 — Create a fresh, empty project

```bash
mkdir ~/govern-check && cd ~/govern-check && git init
git commit --allow-empty -q -m "init"
```

That is the entire per-project setup. No hooks, no contract, no
skills copied here. Contrast with
[tutorial 7](07-copilot-end-to-end.md) which places all three by
hand.

## Step 2 — Open the folder in the Copilot desktop app

Point the Copilot app at `~/govern-check`. Observe:

- **MCP tools visible** — the app's tool drawer lists
  `get_platform_guidelines_for_intent` and `lock_in_plan` under `ppg`,
  loaded from `~/.copilot/mcp-config.json`.
- **Contract loaded** — the platform instructions from
  `~/.copilot/copilot-instructions.md` are visible in the session's
  context (agent-dependent surface).
- **SessionStart fires** — no artefact appears inside the project;
  the session id is recorded via the SessionStore under
  `$XDG_STATE_HOME/ppg/projects/<slug>/session` (the
  `ppg-copilot-guard` binary was invoked by the user-scope hook
  declaration in `~/.copilot/hooks/ppg.json`).

## Step 3 — Run the amplified loop from a single prompt

In Copilot Chat:

> Add a Seka payment method to the checkout service.

**What you should observe**, in order (same choreography as
tutorial 7):

1. Copilot calls `get_platform_guidelines_for_intent`, receives
   ADR-042 (egress proxy) and ADR-070 (frozen paths).
2. Copilot drafts a plan and calls `lock_in_plan`. If the plan lacks
   a `go test` step, the gateway answers `PLAN_REJECTED` and Copilot
   self-corrects in one round-trip.
3. On success: `PLAN_LOCKED`, ticket persisted through the TokenStore
   at `$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<sid>`.
4. Every `Edit`/`Write` in the ticket scope passes silently through
   `ppg-copilot-guard`.

No file was placed by you inside the project to make this work. All
governance travelled with the workstation.

## Step 4 — Trigger the drift refusal

In the same session:

> Also quickly update `internal/auth/login.go`.

**What you should observe**: the user-scope guard denies with
`OUT_OF_PLAN_SCOPE`. Same message as tutorial 7, produced by the same
`ppg-copilot-guard` binary — only its declaration lives at
`~/.copilot/hooks/ppg.json` instead of `.github/hooks/ppg.json` in the
project.

Copilot, following the user-scope contract, does not retry: it either
stays in the ticket scope or offers to re-plan.

## Step 5 — Clean up (project only)

```bash
cd .. && rm -rf ~/govern-check
```

The workstation is unchanged. `~/.copilot/` still holds the MCP
registration, hooks, contract, and skills — ready for the next
project. If you want to fully unconfigure the workstation, follow the
["Rollback" section](../how-to/set-up-a-governed-workstation.md#rollback)
of the how-to.

**✅ Done.** Three shell commands, one prompt, one drift test. The
governance was invisible to the project — which is exactly the point:
in an organizationally-managed workstation, ceremony *is* the friction,
and pushing it into the user profile eliminates it without weakening
the guarantees.
