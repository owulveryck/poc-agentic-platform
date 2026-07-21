# How to gate changes at apply time with `ppg-verify`

> Solves one problem: enforcing the locked plan on surfaces with **no
> in-loop hook** — the `gh copilot` CLI, Cursor, a human at the terminal,
> a CI runner. The in-tool guards (`ppg-guard`, `ppg-copilot-guard`)
> block a bad edit *before* it happens; `ppg-verify` is the deterministic
> backstop that checks the **actual diff** *before it is committed,
> pushed, or merged*.

## What it does

`ppg-verify` reads the active capability ticket from the per-machine
store, computes the git changeset (via `git status --porcelain`, reading
each changed file's current content), and POSTs it to the gateway's
`/verify_changeset`. The gateway runs the same Rego corpus as the plan
linter and the in-loop guards — this time at the **changeset altitude**
(`input.view == "changeset"`, reading `input.changeset.files`). It
verifies three things: the ticket is valid, every changed path is in the
plan's scope, and (when you pass `--plan`) the plan being executed still
matches the one the ticket was issued for.

Exit codes are what a hook or CI step keys on:

| Exit | Meaning |
|---|---|
| `0` | Changeset accepted (`CHANGESET_OK`) |
| `1` | Rejected — violations printed to stderr (`CHANGESET_REJECTED`, `OUT_OF_PLAN_SCOPE`, or `PLAN_SUBSTITUTION`) |
| `2` | Could not run the check (no ticket, gateway unreachable) — **fail closed** |

The distinction between `1` and `2` matters: `1` is a real policy
rejection; `2` means the gate itself could not run and you should treat
it as a hard failure, not a pass.

## Prerequisite

`make install` put `ppg-verify` on `PATH` (it is one of the seven binaries
built by the Makefile). A plan must be locked for the active session so a
ticket exists in the store — see [tutorial 2](../tutorials/02-claude-code-end-to-end.md)
or [tutorial 7](../tutorials/07-copilot-end-to-end.md). The gateway must
be running (default `http://localhost:8765`, override with `PPG_URL` or
`--gateway`).

## Basic usage

```bash
ppg-verify              # verify all working-tree changes vs HEAD
ppg-verify --staged     # verify only the staged changes
ppg-verify --plan plan.json   # also check the plan hash against the ticket
```

`--plan` guards against **plan substitution**: if the plan JSON you point
at hashes to something other than the ticket's `plan_hash` claim, the
gateway answers `PLAN_SUBSTITUTION` (exit 1) — the diff is being applied
under a ticket issued for a different plan. Re-plan through `lock_in_plan`.

## As a pre-commit hook

Block a commit whose staged changes leave the locked scope or break a
content invariant. Scripted install (idempotent, `DRY_RUN=1` to preview):

```bash
make setup-git-backstop            # this repository
GLOBAL=1 make setup-git-backstop   # machine-wide via core.hooksPath
make remove-git-backstop           # undo
```

Or by hand, in `.git/hooks/pre-commit` (or your hook manager):

```bash
#!/bin/sh
ppg-verify --staged || exit 1
```

Posture note: a local git hook is a **cooperative** control —
`git commit --no-verify` skips it. It catches accidental and agent-driven
bypasses of the in-loop guards (Bash writes, other editors), not a hostile
human. The non-bypassable apply-time gate is the same check in CI, below.
The machine-wide variant blocks *every* commit on the machine without a
valid ticket (exit 2, fail closed) — intended for fully-governed
workstations, wrong for mixed human/agent machines.

## As a pre-push hook

Verify the whole working tree before it leaves the machine. In
`.git/hooks/pre-push`:

```bash
#!/bin/sh
ppg-verify || exit 1
```

## In CI

The apply-time gate belongs in CI for the hookless surfaces. Because it
fails closed (`exit 2`), a missing ticket or an unreachable gateway fails
the job rather than silently passing:

```yaml
# e.g. a GitHub Actions step
- name: Enforce the locked plan over the diff
  run: |
    ppg &                       # start (or reach) the gateway
    ppg-verify --plan plan.json
```

## Notes

- **Deletions are included** as `{path, op: "delete"}` with empty content —
  removing a governed file is still a change the changeset policy can
  refuse (match on `f.op == "delete"`). Renames are verified at the new
  path.
- **`PPG_URL`** (or `--gateway`) points at the gateway;
  `--project-dir` / `PPG_PROJECT_DIR` and `--store-root` /
  `PPG_STORE_ROOT` locate the per-machine store, exactly as the guards
  do — so `ppg-verify` reads the ticket the same session locked.
- The rules it runs are authored exactly like any other invariant; write
  a changeset-view (or artifact-view) rule as shown in
  [Enforce a content invariant](enforce-a-content-invariant.md), and both
  the in-loop guard and `ppg-verify` pick it up.
- The endpoint contract is in the [HTTP API reference](../reference/http-api.md#post-verify_changeset);
  the flags in the [gateway CLI reference](../reference/validation-server-cli.md#cmdppg-verify--apply-time--ci-backstop).
