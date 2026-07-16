# Tutorial — govern a live Claude Code session

> **Goal**: wire a stock Claude Code session to the gateway and watch both
> pillars work end to end: the plan is enriched and locked through MCP tools,
> and an out-of-plan edit is blocked by a hook *before* it executes.
>
> Time: ~10 minutes. Prerequisites: [tutorial 0](00-bootstrap.md) completed
> (gateway running on `:8765`; `ppg-guard` and `ppg-mcp-server` on `PATH`;
> `claude mcp list` shows `ppg` as connected).

## Step 1 — Create a scratch target project

The governed session runs in a *separate* project, like any team repository:

```bash
mkdir ~/ppg-demo && cd ~/ppg-demo && git init
mkdir -p internal/payment internal/auth
printf 'package payment\n' > internal/payment/router.go
printf 'package auth\n'    > internal/auth/login.go
```

The platform's session state (active session id + capability ticket) is
persisted under `$XDG_STATE_HOME/ppg/projects/<slug>/` — outside the
project, so nothing needs to be added to `.gitignore`.

`internal/auth/` is one of the frozen legacy paths of ADR-070 — we will use
it to trigger a refusal in step 5.

## Step 2 — Register the hooks

Create `.claude/settings.json` in `~/ppg-demo` (content of
[`settings.example.json`](../../adapters/claudecode/settings.example.json)):

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          { "type": "command", "command": "ppg-guard", "args": [] }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          { "type": "command", "command": "ppg-guard", "args": [] }
        ]
      }
    ]
  }
}
```

The `SessionStart` entry binds tickets to sessions: it records the session
id via the SessionStore (which the MCP server reads at lock time to stamp
the plan) and purges any ticket left by a previous session. Both the
session id and the ticket live under `$XDG_STATE_HOME/ppg/projects/<slug>/` —
outside the project.

## Step 3 — Add the behavioral contract

Copy [`CLAUDE.example.md`](../../adapters/claudecode/CLAUDE.example.md) to
`~/ppg-demo/CLAUDE.md`. It contains the three rules: enrich before planning,
lock before modifying, never retry an `OUT_OF_PLAN_SCOPE` refusal verbatim.

## Step 4 — Run the governed session

Start `claude` in `~/ppg-demo` and prompt:

> Add the Seka payment method to checkout

**What you should observe**, in order:

1. Claude calls `get_platform_guidelines_for_intent` and receives the ADR-042
   invariant (egress proxy) and the ADR-070 frozen paths — the same payload
   you saw in [tutorial 1, step 2](01-first-planning-cycle.md).
2. Claude submits its plan through `lock_in_plan`. If the plan misses a
   `go test` step, the gateway answers `PLAN_REJECTED` with the
   `go_tests_present` violation, and Claude corrects its plan on its own —
   this self-correction in one or two iterations is the expected behavior.

   *Troubleshooting*: if a plan is rejected repeatedly with the same
   violation, the violation message names the exact criterion the policy
   checks (a step whose tool is `go-test` or whose action runs `go test`).
   A rejection loop means the plan does not satisfy that criterion, however
   plausible its steps look; do not guess along other dimensions.
3. On success the response is `PLAN_LOCKED` and the ticket is persisted
   under `$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<sid>`. Inspect its
   claims (adjust `SID` to the session id printed by Claude Code):

   ```bash
   SLUG=$(printf '%s' "$PWD" | base64 | tr '+/' '-_' | tr -d '=')
   SID=<your-session-id>
   TICKET_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/ppg/projects/$SLUG/tickets"
   python3 -c "import base64,json,sys,os; \
   p=open(os.path.join('$TICKET_DIR','$SID')).read().strip().split('.')[1]; \
   print(json.dumps(json.loads(base64.urlsafe_b64decode(p+'='*(-len(p)%4))), indent=2))"
   ```

   ```json
   {
     "session_id": "...",
     "plan_hash": "c8b50f31ca77170b3c0f3f25681554c93a380a666ada779a143f6ff65db0173a",
     "scope": {
       "allow_modify": ["migrations/001_seka.sql", "internal/payment/router.go", "tests/integration_payment_test.go"],
       "allow_tool": ["db-migration-generator", "patch_code", "go-test"]
     },
     "exp": 1783519021,
     "iat": 1783518121
   }
   ```

4. Every `Edit`/`Write` inside the locked scope passes silently through
   `ppg-guard`.

## Step 5 — Trigger the drift refusal

In the same session, prompt:

> Also quickly update internal/auth/login.go

**What you should observe**: the hook blocks the edit *before execution*
(exit code 2), and Claude reads the exact message the guard emits:

```
OUT_OF_PLAN_SCOPE: "internal/auth/login.go" is not part of the locked plan
(allowed: migrations/001_seka.sql, internal/payment/router.go,
tests/integration_payment_test.go). Nothing was modified. If this change is
genuinely needed, re-plan through lock_in_plan.
```

Per the `CLAUDE.md` contract, Claude does not retry the same call: it either
stays within the plan or re-plans through `lock_in_plan`. If no plan is
locked at all, the guard blocks with `No capability ticket for this
session` and points to `lock_in_plan` — the paved road is also the only
road.

One more property to observe: quit and start a **new session** in the same
directory. The `SessionStart` hook purges the previous ticket, and even a
copy of it would be refused (`SESSION_MISMATCH`: the ticket's `session_id`
claim no longer matches the session). A capability dies with the session
that locked it, not only with its
configurable wall-clock TTL.

## Step 6 — Clean up

```bash
rm -rf ~/ppg-demo
```

(The `ppg` MCP registration is user-scope from [tutorial 0](00-bootstrap.md);
leave it in place for the next tutorial. Remove it with
`claude mcp remove ppg --scope user` if you are unwinding the whole setup.)

**✅ Done.** You have seen pillar 1 (amplified planning via MCP) and pillar 2
(deterministic in-tool gating via the hook) run inside an off-the-shelf
agent. The *why* is in
[capability-tickets-and-in-tool-guards.md](../explanation/capability-tickets-and-in-tool-guards.md).
Next step: package this workflow as a governed skill and watch it drive the
session by itself, in
[tutorial 6](06-skill-to-session-end-to-end.md).
