# How to set up a governed workstation

> Install the platform user-wide so every project on this machine is
> governed by default. No per-project ceremony, no forgotten hooks, no
> `.github/hooks/ppg.json` drifting across N repositories. The
> workstation becomes a first-class endpoint of your organization's
> Platform Planning Gateway.
>
> The reader flow demonstrating the result is
> [tutorial 9 (Copilot)](../tutorials/09-copilot-on-governed-workstation.md)
> and [tutorial 10 (Claude Code)](../tutorials/10-claude-on-governed-workstation.md).

## When to use this

Your developer workstation only ever hosts projects that belong to a
single organization running the PPG platform. You want every session on
this machine — main service, side spike, half-finished POC — to be
governed by the organization's ADRs and skills automatically. You're
willing to trade the per-project discoverability that
`.github/hooks/*.json` provides for zero-touch adoption on every new
project.

## Why global is the right default in an organization

The organization has already made the governance decisions. Baking them
into the *machine* rather than into each *project* means:

- **Zero-touch adoption** — a fresh `git init` or `git clone` produces
  a governed workspace the moment the agent opens it. No manual copy
  of hook declarations or contract files.
- **Consistent enforcement across projects** — every side project,
  every scratch dir, every POC is governed by the same ADRs. ADR-070's
  frozen paths are frozen everywhere; compensatory scaffolding is
  applied uniformly.
- **The workstation is an org endpoint** — like corporate email or the
  company VPN, the laptop carries a policy stance. New employees
  onboard by receiving a machine already configured; the platform
  team owns the config, not each individual developer.
- **Trivial upgrades** — a new ADR ships to the gateway, a new skill
  publishes into the registry; both take effect on the next session,
  with no fan-out PR across N repositories.
- **MDM-friendly** — the entire configuration is a handful of files in
  `~/.claude/` and `~/.copilot/`, easy to push, version, and audit at
  the fleet level.
- **The developer stops seeing the machinery** — `.github/hooks/`,
  `CLAUDE.md`, and skill packages disappear from every project's file
  tree. What remains visible in the repo is what the developer works
  on, not the platform's plumbing.

## What has to stay per-project — and only that

Two files, both ephemeral, both session state. They cannot be global
by nature:

- `.ppg-ticket` — a capability JWT bound to one plan in one project.
- `.ppg-session` — the current agent session's id.

Both live inside the project (add them to `.gitignore`), are written
by the platform on demand, and disappear when the session ends.
Everything else moves to `~/`.

## Prerequisite

[Tutorial 0 — Bootstrap the platform on your machine](../tutorials/00-bootstrap.md)
completed. This means:

- `ppg`, `ppg-mcp-server`, `ppg-copilot-guard`, and/or `ppg-guard` are
  on `PATH` (typically `~/.local/bin/`).
- The gateway is running on `:8765` (or is set to auto-start).
- MCP is registered at user scope for at least one agent surface.

## Recipe — Claude Code (user-wide)

Four steps. Steps 1 and 4 are already done by tutorial 0 for the MCP
piece; steps 2 and 3 are the new user-scope files.

### 1. MCP — already user-scope from tutorial 0

Verify:

```bash
claude mcp list          # → ppg   connected   (user)
```

If missing, re-run:

```bash
claude mcp add --scope user ppg \
  --env PPG_URL=http://localhost:8765 -- ppg-mcp-server
```

### 2. Contract — write `~/.claude/CLAUDE.md`

Copy the three-rules contract from the reference example:

```bash
mkdir -p ~/.claude
cp /path/to/poc-agentic-platform/adapters/claudecode/CLAUDE.example.md \
   ~/.claude/CLAUDE.md
```

Every `claude` session on this machine now loads this contract at
startup, regardless of the project. If a project needs to override or
extend it, its own `./CLAUDE.md` takes precedence.

### 3. Hooks — write `~/.claude/settings.json`

```bash
cat > ~/.claude/settings.json <<'EOF'
{
  "hooks": {
    "SessionStart": [
      { "hooks": [
        { "type": "command", "command": "ppg-guard", "args": [] }
      ] }
    ],
    "PreToolUse": [
      { "matcher": "Edit|Write",
        "hooks": [
          { "type": "command", "command": "ppg-guard", "args": [] }
        ] }
    ]
  }
}
EOF
```

From now on, opening ANY project in `claude` triggers `SessionStart`
(which purges any stale `.ppg-ticket` and records the fresh session id
into `.ppg-session`) and gates every `Edit`/`Write` through the ticket
scope.

### 4. Skills — install them user-wide

APM's `--target claude` deploys to `.claude/skills/` in the current
working directory. To make skills user-wide, install them into a
dedicated home-managed directory and let `~/.claude/skills/` be the
canonical location:

```bash
mkdir -p ~/.claude/skills
apm install /path/to/poc-agentic-platform/demo --target claude
# APM will drop skills into ./.claude/skills/ under $CWD; if you ran
# this from $HOME, they now live at ~/.claude/skills/. Alternatively,
# cp -r them from a project install.
```

Verify:

```bash
ls ~/.claude/skills/
# → add-payment-method  design-system  ppg-tutorial
```

## Recipe — GitHub Copilot desktop / `gh copilot` CLI (user-wide)

Same four steps, different filesystem locations under `~/.copilot/`.

### 1. MCP — already user-scope from tutorial 0

Verify (either form works):

```bash
copilot mcp list                       # if the copilot CLI is installed
cat ~/.copilot/mcp-config.json         # otherwise — check for the ppg entry
```

Re-add if missing. **A.** With the CLI:

```bash
copilot mcp add ppg --env PPG_URL=http://localhost:8765 -- ppg-mcp-server
```

**B.** Without the CLI — hand-edit the file (the desktop app reads it
directly):

```bash
mkdir -p ~/.copilot
cat > ~/.copilot/mcp-config.json <<'EOF'
{
  "mcpServers": {
    "ppg": {
      "type": "stdio",
      "command": "ppg-mcp-server",
      "env": { "PPG_URL": "http://localhost:8765" },
      "tools": ["*"]
    }
  }
}
EOF
```

If the file already has other `mcpServers` entries, add `ppg` into
the existing object rather than replacing the whole file.

### 2. Contract — write `~/.copilot/copilot-instructions.md`

The Copilot equivalent of `~/.claude/CLAUDE.md`. Same three-rules
contract; you can preflight it with the platform's invariants for a
generic intent, then append the contract lines:

```bash
mkdir -p ~/.copilot
ppg-preflight -repo any -stack Go,SQL \
  "Follow the amplified planning loop" \
  && mv .github/copilot-instructions.md ~/.copilot/copilot-instructions.md \
  && rmdir .github 2>/dev/null

cat >> ~/.copilot/copilot-instructions.md <<'EOF'

# Platform contract

- Before planning any change, call `get_platform_guidelines_for_intent`
  with the intent and repository context.
- Before modifying anything, submit your plan through `lock_in_plan`.
  A `PreToolUse` hook refuses every edit outside the resulting ticket.
- If a tool refuses with `OUT_OF_PLAN_SCOPE`, do not retry: either
  stay in scope or re-plan.
EOF
```

### 3. Hooks — write `~/.copilot/hooks/ppg.json`

```bash
mkdir -p ~/.copilot/hooks
cat > ~/.copilot/hooks/ppg.json <<'EOF'
{
  "hooks": {
    "SessionStart": [
      { "type": "command", "command": "ppg-copilot-guard", "timeoutSec": 5 }
    ],
    "PreToolUse": [
      { "type": "command", "command": "ppg-copilot-guard", "timeoutSec": 5 }
    ]
  }
}
EOF
```

Project-level `.github/hooks/*.json` still takes precedence when
present, so a project can add a content-scope hook (e.g. the
`design-guard.sh` shipped by the `design-system` skill) on top of the
user-scope gates.

### 4. Skills — install them user-wide

The user-scope directory for Copilot-side skills is
`~/.copilot/skills/` on recent versions; on older ones, APM may only
deploy to `./.agents/skills/`. Best-effort recipe:

```bash
mkdir -p ~/.copilot/skills
# Preferred: apm --target copilot pointed at the user location.
# Fallback: install per-project and cp -r into ~/.copilot/skills/.
```

If your Copilot version does not read `~/.copilot/skills/`, keep the
skills per-project (`apm install ... --target copilot` inside each
project). Only the MCP + hooks + contract need to be user-wide for the
governance to travel with the machine.

## Recipe — VS Code Copilot Chat (partial)

VS Code Copilot Chat's MCP config is **workspace-scoped only**
(`.vscode/mcp.json`). Full user-wide deployment is not achievable
here today; `.vscode/mcp.json` remains a per-project artefact for VS
Code users. Two workarounds:

- Symlink each project's `.vscode/mcp.json` to a canonical file in
  `~/`.
- Prefer the Copilot desktop app or `gh copilot` CLI for genuinely
  user-wide governance.

Hooks and instructions in VS Code Copilot Chat can be moved user-side
via the `chat.hookFilesLocations` and `chat.instructionsFilesLocations`
user settings — configure them to include a directory under `~/`.

## What you lose (be honest)

- **Discoverability from the project** — someone reading a repo can no
  longer answer "what governs this codebase?" by inspecting its files.
  Mitigation: drop a one-line `.github/PLATFORM.md` in each governed
  project pointing at the org's platform docs.
- **Reproducibility across machines** — every developer laptop needs
  the same setup. Mitigation: MDM push, or a `bootstrap.sh` in the
  org's onboarding docs (which is essentially this how-to,
  scripted).
- **Per-project opt-out is harder** — a project that shouldn't be
  governed (e.g., an unrelated OSS side project on the same laptop)
  has to actively neutralize the user-level config. This is the
  scenario where a hybrid deployment fits: leave the machine
  governed by default, and let opt-out projects unregister the hooks
  locally via an empty `.claude/settings.json` or equivalent.

## Verify

The single test that proves the workstation is governed:

```bash
mkdir /tmp/govern-check && cd /tmp/govern-check && git init
# Do NOT create .github/hooks/, .github/copilot-instructions.md,
# CLAUDE.md, or any per-project config.
```

Open the folder in your agent (Copilot desktop, `claude`, ...) and
ask it to edit any file. Expected:

- `SessionStart` hook fires, `.ppg-session` appears in the project.
- The first `Edit` attempt is refused with `No capability ticket
  found` (from the user-scope guard).
- The agent, following the user-scope contract, calls
  `lock_in_plan` first — the paved road is the only road.

No file placed manually in the project. The full amplified loop
works purely from what lives under `~/`.

For the interactive end-to-end demonstration in Copilot see
[tutorial 9](../tutorials/09-copilot-on-governed-workstation.md); for
Claude Code see
[tutorial 10](../tutorials/10-claude-on-governed-workstation.md).

## Rollback

Deployment recipes always ship with their inverse.

### Claude Code

```bash
claude mcp remove ppg --scope user
rm ~/.claude/CLAUDE.md
rm ~/.claude/settings.json
rm -rf ~/.claude/skills
```

### Copilot desktop / CLI

```bash
# Remove the ppg MCP entry — either form works:
copilot mcp remove ppg     # with the CLI
# OR, without the CLI, hand-edit ~/.copilot/mcp-config.json to delete
# the "ppg" key from the mcpServers object:
jq 'del(.mcpServers.ppg)' ~/.copilot/mcp-config.json \
  > /tmp/mcp-config.json && mv /tmp/mcp-config.json ~/.copilot/mcp-config.json

rm ~/.copilot/copilot-instructions.md
rm -rf ~/.copilot/hooks
rm -rf ~/.copilot/skills
```

The gateway (`ppg` process) and machine binaries under `~/.local/bin/`
survive — remove them separately if fully unwinding the platform.
