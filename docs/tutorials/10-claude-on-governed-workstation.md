# Tutorial 10 — Claude Code on a governed workstation

> **Goal**: the Claude Code sibling of
> [tutorial 9](09-copilot-on-governed-workstation.md). With the
> workstation configured user-wide, spinning up a governed `claude`
> session on a brand-new project takes three shell commands and one
> prompt. No `.claude/settings.json`, no project `CLAUDE.md`, no APM
> per-project — the whole ceremony from tutorial 2 disappears.
>
> Time: ~5 minutes.
> Prerequisites:
> - [Tutorial 0 — bootstrap](00-bootstrap.md) completed.
> - [How-to — set up a governed workstation](../how-to/set-up-a-governed-workstation.md)
>   applied for the Claude Code recipe: user-scope MCP
>   (`claude mcp list` shows `ppg`), hooks in `~/.claude/settings.json`,
>   contract in `~/.claude/CLAUDE.md`, and optionally skills in
>   `~/.claude/skills/`.

## Step 1 — Create a fresh, empty project

```bash
mkdir ~/govern-check && cd ~/govern-check && git init
git commit --allow-empty -q -m "init"
```

Zero per-project files at all — session state lives under
`$XDG_STATE_HOME/ppg/projects/<slug>/`, outside the project. Contrast
with [tutorial 2](02-claude-code-end-to-end.md) which places hooks and
`CLAUDE.md` by hand.

## Step 2 — Launch `claude` in the folder

```bash
claude
```

Observe:

- **MCP tools available** — inside the session, the tools
  `get_platform_guidelines_for_intent` and `lock_in_plan` are exposed
  from the user-scope `ppg` server (verify externally with
  `claude mcp list`).
- **Contract loaded** — the three-rules contract from
  `~/.claude/CLAUDE.md` is part of the session's system prompt (Claude
  Code merges user-scope `CLAUDE.md` with the project one, if any).
- **SessionStart fires** — no artefact appears in the project; the
  session id is recorded via the SessionStore under
  `$XDG_STATE_HOME/ppg/projects/<slug>/session`. The `ppg-guard` binary
  was invoked by the user-scope hook declaration in
  `~/.claude/settings.json`; it purges any stale tickets from the
  TokenStore and records the fresh session id.

## Step 3 — Run the amplified loop from a single prompt

In the `claude` prompt:

> Add a Seka payment method to the checkout service.

**What you should observe**, in order (same choreography as
tutorial 2):

1. Claude calls `get_platform_guidelines_for_intent`, receives
   ADR-042 and ADR-070.
2. Claude submits its plan through `lock_in_plan`. If the plan lacks
   a `go test` step, the gateway answers `PLAN_REJECTED` with the
   `go_tests_present` violation, and Claude corrects in one
   round-trip.
3. On success: `PLAN_LOCKED`, ticket persisted through the TokenStore
   at `$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<sid>`.
4. Every `Edit`/`Write` inside the scope passes silently through
   `ppg-guard`.

No file was placed by you inside the project to make this work.

## Step 4 — Trigger the drift refusal

In the same session:

> Also quickly update `internal/auth/login.go`.

**What you should observe**: the hook blocks the edit before
execution (`exit 2`), and Claude reads:

```
OUT_OF_PLAN_SCOPE: "internal/auth/login.go" is not part of the locked
plan (allowed: migrations/001_seka.sql, internal/payment/router.go,
tests/integration_payment_test.go). Nothing was modified. If this
change is genuinely needed, re-plan through lock_in_plan.
```

Per the user-scope `CLAUDE.md` contract, Claude does not retry: it
either stays within the plan or re-plans through `lock_in_plan`.

## Step 5 — Clean up (project only)

```bash
/exit    # inside claude
cd .. && rm -rf ~/govern-check
```

The workstation is unchanged. `~/.claude/` still holds the MCP
registration, hooks, contract, and skills — ready for the next
project. If you want to fully unconfigure the workstation, follow the
["Rollback" section](../how-to/set-up-a-governed-workstation.md#rollback)
of the how-to.

**✅ Done.** Same three commands, same one prompt, same drift test
as tutorial 9 — different agent surface, identical result. The
framework doesn't care which agent surface the workstation was
configured for; the governance travels with the machine.
