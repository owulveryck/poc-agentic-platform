# How to set up a governed workstation

> Install the platform user-wide so every project on this machine is
> governed by default. No per-project ceremony, no forgotten hooks, no
> `.github/hooks/ppg.json` drifting across N repositories. The
> workstation becomes a first-class endpoint of your organization's
> governance harness.
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
- **Trivial upgrades** — a new ADR ships to the validation server, a new skill
  publishes into the registry; both take effect on the next session,
  with no fan-out PR across N repositories.
- **MDM-friendly** — the entire configuration is a handful of files
  either under `~/.claude/` / `~/.copilot/` (user scope, per-user push)
  or under the OS-level managed-settings path (managed scope, root-owned;
  `allowManagedHooksOnly` closes the settings-edit vector — see the
  tamper-model caveat in recipe (A) for the binary and environment vectors
  it does *not* close). See
  [(A) Managed scope](#a-managed-scope--recommended-for-it-managed-fleets)
  below.
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
- The validation server is running on `:8765` (or is set to auto-start).
- MCP is registered at user scope for at least one agent surface.

## Recipe — Claude Code

Two options, same end-to-end UX, different tamper-proofing guarantee. Pick
the one that matches your deployment.

### (A) Managed scope — recommended for IT-managed fleets

Claude Code's [scope precedence](https://code.claude.com/docs/en/settings)
is *Managed > CLI > Local > Project > User*. Installing hooks at **managed
scope** (root-owned) with `allowManagedHooksOnly: true` makes user, project,
and plugin hooks silently ignored — so a repository cannot ship a
`.claude/settings.json` that overrides the platform hooks. This closes
[tutorial 12 A10](../tutorials/12-bypassing-the-gateway.md#a10--disable-the-guard-by-editing-its-own-config)
at the settings layer; a user-scope install only guards it softly.

Managed scope does **not** by itself make the guard tamper-proof. The hook
executes whatever binary its command points at, and the guard reads its
environment. For a hostile-user threat model, additionally:

1. install the binaries in a root-owned directory —
   `sudo BINDIR=/usr/local/bin make install` — so the user cannot replace
   `ppg-guard` itself (the default `~/.local/bin` is user-writable);
2. pin `PPG_URL` in the managed hook command so content verification
   cannot be re-pointed at a rogue validation server; `PPG_TICKET_SECRET` and
   `PPG_STORE_ROOT` follow the same argument.

The managed setup script refuses to install (unless `FORCE=1`) when the
resolved `ppg-guard` binary is user-writable, and prints the pinning
recipe.

Requires root. One command:

```bash
sudo make setup-claude-code-managed
```

Preview without touching the disk (no root required):

```bash
DRY_RUN=1 make setup-claude-code-managed
```

What that writes, per OS:

| OS | Path |
|---|---|
| macOS | `/Library/Application Support/ClaudeCode/managed-settings.json` |
| Linux / WSL | `/etc/claude-code/managed-settings.json` |
| Windows | `C:\Program Files\ClaudeCode\managed-settings.json` (install by hand — see the shape in [`adapters/claudecode/managed-settings.example.json`](../../adapters/claudecode/managed-settings.example.json)) |

Guarantees, same as the user-scope script:

- **Surgical merge** — any non-ppg policy already in the file
  (e.g. `permissions`, IT-authored hooks) is preserved. If
  `allowManagedHooksOnly` is already set to `false`, it is *not* flipped
  without `FORCE=1` (respects an explicit operator choice).
- **Backup** on every modifying write to `<file>.bak-YYYYMMDDHHMMSS`.
- **Absolute path** to `ppg-guard` resolved at install time.
- **File mode `0644 root:root`** so Claude Code (running as the user) can
  read it.

MCP registration stays at user scope (`~/.claude.json`) — it's not a
policy, and pushing MCP config to root scope is not something Claude Code
supports uniformly across surfaces. Run the user-scope MCP install once
per user account:

```bash
make setup-claude-code       # for the MCP entry only; hooks come from managed
```

**MDM alternatives.** For fleet management without shell access:

- macOS (Jamf, Kandji, Mosyle, Intune): push a configuration profile for
  the `com.anthropic.claudecode` preferences domain — same JSON keys as
  `managed-settings.json`.
- Windows (Group Policy / Intune): set `HKLM\SOFTWARE\Policies\ClaudeCode`
  → `Settings` with the JSON payload.
- Split ownership (IT vs platform team): use the drop-in directory
  `managed-settings.d/*.json` next to `managed-settings.json`; files are
  merged alphabetically. Prefix with a two-digit sort key (e.g.
  `50-ppg.json`) to control ordering.

**Verify the hard refusal** (manual smoke test on a scratch VM/container):

1. `sudo make setup-claude-code-managed`.
2. In a scratch project, drop `.claude/settings.json` with a `PreToolUse`
   matcher `Edit|Write` calling a benign `echo` — a fake ppg-bypass.
3. Also inject the same entry into `~/.claude/settings.json`.
4. Open `claude`, attempt an `Edit` with no locked plan.
5. Expected: block fires with `No capability ticket for this session`;
   project- *and* user-scope hooks are silently ignored.
6. `sudo make remove-claude-code-managed`; reopen; the project-scope
   bypass now runs (guard gone, project hooks re-enabled).

### (B) User scope — dev workstation, no root

Use this when you don't own the machine's root, or on a personal machine.
**Trade-off**: a project's `.claude/settings.json` can override user-scope
hooks and disable the guard for that project (see
[tutorial 12 A10](../tutorials/12-bypassing-the-gateway.md#a10--disable-the-guard-by-editing-its-own-config) —
soft-guarded, not hard-guarded). For governed fleets, prefer (A).

#### 1. MCP + hooks — one command

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

#### 2. Contract — write `~/.claude/CLAUDE.md`

Copy the three-rules contract from the reference example:

```bash
mkdir -p ~/.claude
cp /path/to/poc-agentic-platform/adapters/claudecode/CLAUDE.example.md \
   ~/.claude/CLAUDE.md
```

Every `claude` session on this machine now loads this contract at
startup, regardless of the project. If a project needs to override or
extend it, its own `./CLAUDE.md` takes precedence.

#### About the session state

Opening ANY project in `claude` now triggers `SessionStart`, which
purges any stale tickets from the per-project TokenStore under
`$XDG_STATE_HOME/ppg/projects/<slug>/tickets/` and records the fresh
session id in the SessionStore. `Edit`/`Write` calls then get gated
through the ticket scope.

#### Manual alternative (if you'd rather see the wiring)

The Make target is a wrapper over `scripts/setup-claude-code.sh`. Read
that script for the exact JSON writes; the target files are
`~/.claude.json` (`mcpServers.ppg`) and `~/.claude/settings.json`
(`hooks.SessionStart[]` + `hooks.PreToolUse[]` with matcher `Edit|Write`),
using absolute paths to `ppg-mcp-server` and `ppg-guard`. For the
managed-scope variant, see `scripts/setup-claude-code-managed.sh`
(same shape at the OS-level managed-settings path).

### Skills — install them user-wide (both scopes)

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

> **No managed-scope equivalent today.** GitHub Copilot has no documented
> counterpart to Claude Code's `allowManagedHooksOnly`.
> `~/.copilot/hooks/ppg.json` is user-writable and can be deleted or
> edited by the account that runs Copilot. OS-level file-locking
> (`chattr +i` on Linux, SIP-protected paths on macOS) is **not** a
> substitute — it breaks Copilot updates and does not prevent replacement
> at the parent-directory level. For the **strongest governance today,
> use Claude Code at managed scope** (see the (A) recipe above) — and note
> that even managed scope only resists a hostile user once the operational
> hardening there (root-owned binaries, pinned `PPG_*` env) is applied;
> treat Copilot governance as best-effort until GitHub ships a managed-hooks
> feature. The apply-time backstop (`ppg-verify` in pre-commit / CI)
> catches escapes on any surface, Copilot included — see
> [gate-changes-at-apply-time](gate-changes-at-apply-time.md).

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
already sends every edit to the validation server's `/verify_artifact`, so an ADR's
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
# Managed scope (only if you installed via (A) above):
sudo make remove-claude-code-managed   # DRY_RUN=1 to preview (no sudo needed)

# User scope (MCP + user-scope hooks):
make remove-claude-code        # DRY_RUN=1 to preview
rm -f ~/.claude/CLAUDE.md      # if you deployed the contract
rm -rf ~/.claude/skills        # if you deployed skills user-wide
```

`make remove-claude-code-managed` strips only the ppg-guard hook entries
from the OS-level `managed-settings.json`; if the resulting file contains
no other policy, `allowManagedHooksOnly` is dropped and the file is
removed entirely (a timestamped backup is taken first). Non-ppg policy
(other hooks, permissions, IT-authored settings) is always preserved.

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

The validation server (`ppg` process) and machine binaries under `~/.local/bin/`
survive too — `make uninstall` from the checkout removes the binaries.
