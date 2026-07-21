#!/usr/bin/env bash
# Remove the ppg-verify git backstop installed by setup-git-backstop.sh.
# Repo-local mode: deletes .git/hooks/pre-commit if it is entirely ppg's
# (refuses if it contains anything else — remove the ppg line by hand).
# GLOBAL=1: unsets core.hooksPath if it points at the ppg hooks dir and
# deletes that directory.
#
# Env: DRY_RUN=1 (preview), GLOBAL=1.

set -euo pipefail
source "$(dirname "$0")/lib.sh"

: "${GLOBAL:=0}"

if [ "$GLOBAL" = "1" ]; then
    HOOKS_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/ppg/git-hooks"
    current=$(git config --global core.hooksPath 2>/dev/null || true)
    if [ "$current" = "$HOOKS_DIR" ]; then
        if [ "$DRY_RUN" = "1" ]; then
            log "DRY_RUN: would unset core.hooksPath and delete $HOOKS_DIR"
        else
            git config --global --unset core.hooksPath
            rm -rf "$HOOKS_DIR"
            ok "unset core.hooksPath and removed $HOOKS_DIR"
        fi
    else
        log "core.hooksPath is '${current:-unset}', not ppg's — nothing to do"
    fi
    exit 0
fi

REPO_DIR=${1:-$PWD}
GIT_DIR=$(git -C "$REPO_DIR" rev-parse --git-dir 2>/dev/null) \
    || err "$REPO_DIR is not a git repository"
case "$GIT_DIR" in
    /*) HOOK="$GIT_DIR/hooks/pre-commit" ;;
    *)  HOOK="$REPO_DIR/$GIT_DIR/hooks/pre-commit" ;;
esac

if [ ! -e "$HOOK" ]; then
    log "no pre-commit hook at $HOOK — nothing to do"
    exit 0
fi
if ! grep -q 'ppg apply-time backstop' "$HOOK"; then
    log "$HOOK does not contain the ppg backstop — nothing to do"
    exit 0
fi
# Only auto-delete a hook that is entirely ours (header written by setup).
if head -2 "$HOOK" | grep -q 'ppg apply-time backstop'; then
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would delete $HOOK"
    else
        backup "$HOOK"
        rm -f "$HOOK"
        ok "removed $HOOK"
    fi
else
    warn "$HOOK contains non-ppg content; remove the ppg-verify lines by hand (backup made none)"
fi
