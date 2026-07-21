#!/usr/bin/env bash
# Wire ppg-verify as a git pre-commit hook — the apply-time backstop for
# surfaces the in-loop guards cannot see (Bash writes, other editors, a
# human at the terminal). See docs/how-to/gate-changes-at-apply-time.md.
#
# Default: installs .git/hooks/pre-commit in the repository at $PWD (or the
# directory passed as $1). With GLOBAL=1: sets `git config --global
# core.hooksPath` to a ppg-managed hooks directory whose pre-commit chains
# to each repository's own hook first — machine-wide coverage, one command.
#
# Posture note (be honest about what this is): a local git hook is a
# *cooperative* control — `git commit --no-verify` skips it. It exists to
# catch accidental and agent-driven bypasses of the in-loop guards, not a
# hostile human. The non-bypassable apply-time gate for hostile actors is
# the same `ppg-verify` run in CI, where --no-verify does not exist.
#
# Env: DRY_RUN=1 (preview), FORCE=1 (append to an existing non-ppg hook),
#      GLOBAL=1 (core.hooksPath machine-wide mode).

set -euo pipefail
source "$(dirname "$0")/lib.sh"

VERIFY=$(need_binary ppg-verify)
: "${GLOBAL:=0}"

hook_body() {
    cat <<EOF
#!/bin/sh
# ppg apply-time backstop — installed by scripts/setup-git-backstop.sh.
# Verifies the staged diff against the locked plan's scope and the policy
# corpus (changeset altitude). Exit 2 = the check could not run (no ticket,
# validation server down): fail closed, the commit is blocked too.
$VERIFY --staged || exit 1
EOF
}

install_hook_file() {
    local hook=$1
    if [ -e "$hook" ]; then
        if grep -q 'ppg-verify\|ppg apply-time backstop' "$hook"; then
            ok "already installed: $hook"
            return 0
        fi
        if [ "$FORCE" != "1" ]; then
            err "$hook exists and is not ppg's — append '$VERIFY --staged || exit 1' yourself, or re-run with FORCE=1 to append it"
        fi
        if [ "$DRY_RUN" = "1" ]; then
            log "DRY_RUN: would append the ppg-verify line to $hook"
            return 0
        fi
        backup "$hook"
        printf '\n# ppg apply-time backstop — appended by scripts/setup-git-backstop.sh\n%s --staged || exit 1\n' "$VERIFY" >> "$hook"
        ok "appended ppg-verify to $hook"
        return 0
    fi
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would write $hook:"
        hook_body
        return 0
    fi
    mkdir -p "$(dirname "$hook")"
    hook_body > "$hook"
    chmod 0755 "$hook"
    ok "installed $hook"
}

if [ "$GLOBAL" = "1" ]; then
    HOOKS_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/ppg/git-hooks"
    HOOK="$HOOKS_DIR/pre-commit"
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would write $HOOK and set 'git config --global core.hooksPath $HOOKS_DIR'"
    else
        mkdir -p "$HOOKS_DIR"
        cat > "$HOOK" <<EOF
#!/bin/sh
# ppg apply-time backstop (global core.hooksPath) — installed by
# scripts/setup-git-backstop.sh. Chains to the repository's own pre-commit
# hook first so per-repo hooks keep working.
repo_hook="\$(git rev-parse --git-path hooks/pre-commit 2>/dev/null)"
if [ -n "\$repo_hook" ] && [ -x "\$repo_hook" ]; then
    "\$repo_hook" || exit 1
fi
$VERIFY --staged || exit 1
EOF
        chmod 0755 "$HOOK"
        current=$(git config --global core.hooksPath 2>/dev/null || true)
        if [ -n "$current" ] && [ "$current" != "$HOOKS_DIR" ] && [ "$FORCE" != "1" ]; then
            err "core.hooksPath is already '$current'; re-run with FORCE=1 to point it at $HOOKS_DIR (your existing hooks dir would stop applying)"
        fi
        git config --global core.hooksPath "$HOOKS_DIR"
        ok "installed $HOOK and set core.hooksPath (all repositories on this machine)"
    fi
    warn "note: every commit on this machine now requires a locked plan's ticket (or exits 2, fail closed)."
    warn "for purely-human repositories, unset with 'git config --global --unset core.hooksPath' or scripts/remove-git-backstop.sh."
    exit 0
fi

REPO_DIR=${1:-$PWD}
GIT_DIR=$(git -C "$REPO_DIR" rev-parse --git-dir 2>/dev/null) \
    || err "$REPO_DIR is not a git repository (pass the repo as \$1, or GLOBAL=1 for machine-wide)"
case "$GIT_DIR" in
    /*) HOOK="$GIT_DIR/hooks/pre-commit" ;;
    *)  HOOK="$REPO_DIR/$GIT_DIR/hooks/pre-commit" ;;
esac
install_hook_file "$HOOK"
log "CI is the non-bypassable half — add the snippet from docs/how-to/gate-changes-at-apply-time.md to your pipeline."
