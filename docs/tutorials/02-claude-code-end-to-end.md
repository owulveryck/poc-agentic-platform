# Tutorial — govern a live Claude Code session

> **Goal**: wire a stock Claude Code session to the gateway and watch both
> pillars work end to end: the plan is enriched and locked through MCP tools,
> and an out-of-plan edit is blocked by a hook *before* it executes.
>
> Time: ~15 minutes. Prerequisites: Go 1.25+, the
> [Claude Code CLI](https://code.claude.com/) installed, this repository
> cloned (the paths below assume `/path/to/poc-agentic-platform`).

## Step 1 — Start the gateway

From the `poc-agentic-platform` root:

```bash
go run ./cmd/ppg -addr :8765
```

Wait for `Platform Planning Gateway listening on :8765`.

## Step 2 — Create a scratch target project

The governed session runs in a *separate* project, like any team repository:

```bash
mkdir ~/ppg-demo && cd ~/ppg-demo && git init
echo ".ppg-ticket" >> .gitignore
mkdir -p internal/payment internal/auth
printf 'package payment\n' > internal/payment/router.go
printf 'package auth\n'    > internal/auth/login.go
```

`internal/auth/` is one of the frozen legacy paths of ADR-070 — we will use
it to trigger a refusal later.

## Step 3 — Build and install the guard

```bash
go build -o ~/.local/bin/ppg-guard /path/to/poc-agentic-platform/adapters/claudecode/guard
```

(`~/.local/bin` must be on your `PATH`.)

## Step 4 — Register the MCP server

Still in `~/ppg-demo`:

```bash
claude mcp add ppg --env PPG_URL=http://localhost:8765 \
  -- go run /path/to/poc-agentic-platform/adapters/claudecode/mcpserver
```

**What you should observe**: `claude mcp list` shows `ppg` as connected, and
inside a session the tools `get_platform_guidelines_for_intent` and
`lock_in_plan` are available.

## Step 5 — Register the hook

Create `.claude/settings.json` in `~/ppg-demo` (content of
[`settings.example.json`](../../adapters/claudecode/settings.example.json)):

```json
{
  "hooks": {
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

## Step 6 — Add the behavioral contract

Copy [`CLAUDE.example.md`](../../adapters/claudecode/CLAUDE.example.md) to
`~/ppg-demo/CLAUDE.md`. It contains the three rules: enrich before planning,
lock before modifying, never retry an `OUT_OF_PLAN_SCOPE` refusal verbatim.

## Step 7 — Run the governed session

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
3. On success the response is `PLAN_LOCKED` and the ticket lands in
   `.ppg-ticket`. Inspect its claims:

   ```bash
   python3 -c "import base64,json; p=open('.ppg-ticket').read().strip().split('.')[1]; \
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

## Step 8 — Trigger the drift refusal

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
locked at all, the guard blocks with
`No capability ticket found (.ppg-ticket)` and points to `lock_in_plan` —
the paved road is also the only road.

## Step 9 — Clean up

```bash
claude mcp remove ppg
rm -rf ~/ppg-demo
```

**✅ Done.** You have seen pillar 1 (amplified planning via MCP) and pillar 2
(deterministic in-tool gating via the hook) run inside an off-the-shelf
agent. The *why* is in
[capability-tickets-and-in-tool-guards.md](../explanation/capability-tickets-and-in-tool-guards.md).
