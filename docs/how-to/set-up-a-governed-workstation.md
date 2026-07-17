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

Nothing. All ephemeral session state (the capability JWT and the active
session id) is written by the platform under a per-machine state root
outside the project — `$XDG_STATE_HOME/ppg/projects/<slug>/` by default
(see [capability-ticket.md — Storage layout](../reference/capability-ticket.md#storage-layout)).
Projects stay clean: no `.ppg-ticket`, no `.ppg-session`, no additions
to `.gitignore`. Everything the workstation needs lives under `~/`.

## Prerequisite

[Tutorial 0 — Bootstrap the platform on your machine](../tutorials/00-bootstrap.md)
completed. This means:

- `ppg`, `ppg-mcp-server`, `ppg-copilot-guard`, and/or `ppg-guard` are
  on `PATH` (typically `~/.local/bin/`).
- The gateway is running on `:8765` (or is set to auto-start).
- MCP is registered at user scope for at least one agent surface.

## Recipe — Claude Code (user-wide)

### 1. MCP + hooks — one command

From the `poc-agentic-platform` checkout:

```bash
make setup-claude-code
```

That single command:

- Registers `ppg` under user-scope `mcpServers` in `~/.claude.json` (idempotent — skips if already present; pass `FORCE=1` to overwrite a differing entry).
- Merges the `SessionStart` and `PreToolUse` (matcher `Edit|Write`) hook entries into `~/.claude/settings.json` **surgically** — any non-ppg hook you already had is preserved.
- Backs up each file it actually modifies to `<file>.bak-YYYYMMDDHHMMSS`.
- Uses **absolute paths** to the binaries (via `command -v`), which is what GUI-launched agents need — they don't inherit shell PATH.

Preview without touching the disk:

```bash
DRY_RUN=1 make setup-claude-code
```

Verify:

```bash
claude mcp list          # → ppg   connected   (user)
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

### About the session state

Opening ANY project in `claude` now triggers `SessionStart`, which
purges any stale tickets from the per-project TokenStore under
`$XDG_STATE_HOME/ppg/projects/<slug>/tickets/` and records the fresh
session id in the SessionStore. `Edit`/`Write` calls then get gated
through the ticket scope.

### Manual alternative (if you'd rather see the wiring)

The Make target is a wrapper over `scripts/setup-claude-code.sh`. Read
that script for the exact JSON writes; the target files are
`~/.claude.json` (`mcpServers.ppg`) and `~/.claude/settings.json`
(`hooks.SessionStart[]` + `hooks.PreToolUse[]` with matcher `Edit|Write`),
using absolute paths to `ppg-mcp-server` and `ppg-guard`.

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

Re-add if missing — one command from the `poc-agentic-platform`
checkout, same guarantees as the Claude Code recipe (idempotent, backup
on modify, absolute paths, non-ppg entries preserved):

```bash
make setup-github-copilot     # DRY_RUN=1 to preview, FORCE=1 to overwrite
```

That target writes `mcpServers.ppg` in `~/.copilot/mcp-config.json` and
the ppg-dedicated hook file `~/.copilot/hooks/ppg.json` (rewritten
whole; a backup is taken first if the file already existed).

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

### 3. Hooks — already written by step 1

`make setup-github-copilot` in step 1 wrote `~/.copilot/hooks/ppg.json`
for you (dedicated file, safe to rewrite; a backup was taken if it
already existed).

Content invariants do not need a project hook: the user-scope guard
already sends every edit to the gateway's `/verify_artifact`, so an ADR's
artifact-view rule (e.g. `examples/adr/ADR-090.rego` for the `design-system`
skill) is enforced machine-wide. Project-level `.github/hooks/*.json`
still takes precedence when present, so a project can add a bespoke hook
on top of the user-scope gates for the rare check that cannot be
expressed in Rego.

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

- `SessionStart` hook fires; `ls -la` in the project shows **no**
  `.ppg-session` / `.ppg-ticket` file (the session id is now recorded
  under `$XDG_STATE_HOME/ppg/projects/<slug>/session`).
- The first `Edit` attempt is refused with `No capability ticket for
  this session` (from the user-scope guard).
- The agent, following the user-scope contract, calls
  `lock_in_plan` first — the paved road is the only road.

No file placed manually in the project. The full amplified loop
works purely from what lives under `~/`.

For the interactive end-to-end demonstration in Copilot see
[tutorial 9](../tutorials/09-copilot-on-governed-workstation.md); for
Claude Code see
[tutorial 10](../tutorials/10-claude-on-governed-workstation.md).

## Rollback

Deployment recipes always ship with their inverse. Same guarantees as
setup: surgical removal (only ppg entries), timestamped backups, no
touch to non-ppg config.

### Claude Code

```bash
make remove-claude-code        # DRY_RUN=1 to preview
rm -f ~/.claude/CLAUDE.md      # if you deployed the contract
rm -rf ~/.claude/skills        # if you deployed skills user-wide
```

`make remove-claude-code` clears ppg from **global config and the current
project only** — other projects on this machine are never touched. It removes
the top-level `mcpServers.ppg` (user scope), the current project's
`projects.<cwd>.mcpServers.ppg` (local scope) and the stale `ppg` entry in its
`enabled`/`disabledMcpjsonServers` approval lists, the ppg-guard hook entries
in `~/.claude/settings.json`, and `mcpServers.ppg` in `~/.mcp.json` (the
project-scoped `.mcp.json` that otherwise makes Claude prompt "New MCP server
found, use it?"). Non-ppg config is untouched and a backup is taken before
each write. A `.mcp.json` **committed inside the current project repo** is
reported (not edited) with the exact key to remove yourself. Run it from
inside each project you want cleaned.

### Copilot desktop / CLI

```bash
make remove-github-copilot
rm -f ~/.copilot/copilot-instructions.md    # if you deployed the contract
rm -rf ~/.copilot/skills                     # if you deployed skills user-wide
```

`make remove-github-copilot` unregisters `mcpServers.ppg` in
`~/.copilot/mcp-config.json` (other MCP servers preserved) and deletes
the ppg-dedicated hook file `~/.copilot/hooks/ppg.json` (backup taken
first). It also **detects and lists** the current project's repo-committed ppg
registrations — `.github/hooks/ppg.json`, a ppg-guard hook in
`.claude/settings.json`, and `servers.ppg` in `.vscode/mcp.json` — with the
exact removal step for each (these are often git-committed, so they are
reported, not edited). Only the current directory is inspected; run it from
inside each project you want cleaned.

### State directory

The per-machine state dir (`$XDG_STATE_HOME/ppg`) survives — it's data,
not config. Wipe manually if fully unwinding:

```bash
rm -rf "${XDG_STATE_HOME:-$HOME/.local/state}/ppg"
```

The gateway (`ppg` process) and machine binaries under `~/.local/bin/`
survive too — `make uninstall` from the checkout removes the binaries.
