#!/usr/bin/env bash
# Install the ppg-guard hooks at MANAGED scope
# (/Library/Application Support/ClaudeCode/managed-settings.json on macOS,
# /etc/claude-code/managed-settings.json on Linux). Sets
# allowManagedHooksOnly:true so user/project/plugin hooks are ignored —
# closes tutorial 12 A10 (edit ~/.claude/settings.json to remove the hook).
#
# Requires root. Preview with DRY_RUN=1 (no sudo needed). Test path can be
# overridden with PPG_MANAGED_SETTINGS_PATH.
#
# Merges surgically: existing non-ppg policy is preserved; only ppg-guard
# entries are added. Backs up on any change.
#
# Env: DRY_RUN=1 (preview, no root required), FORCE=1 (overwrite a
#      differing allowManagedHooksOnly setting), PPG_MANAGED_SETTINGS_PATH.

set -euo pipefail
source "$(dirname "$0")/lib.sh"

GUARD=$(need_binary ppg-guard)
TARGET=$(managed_settings_path)
need_root

# --- Trust chain: a root-owned managed config must not execute a binary the
# non-root user can replace — `cp /bin/true ~/.local/bin/ppg-guard` would
# defeat allowManagedHooksOnly entirely. Refuse unless FORCE=1.
trust_verdict=$(python3 - "$GUARD" <<'PY'
import os, stat, sys
p = sys.argv[1]
st = os.stat(p)
problems = []
if st.st_uid != 0:
    problems.append(f"owned by uid {st.st_uid}, not root")
if st.st_mode & (stat.S_IWGRP | stat.S_IWOTH):
    problems.append("group/other-writable")
d = os.path.dirname(p)
dst = os.stat(d)
if dst.st_uid != 0:
    problems.append(f"parent dir {d} owned by uid {dst.st_uid}, not root")
elif dst.st_mode & (stat.S_IWGRP | stat.S_IWOTH) and not dst.st_mode & stat.S_ISVTX:
    problems.append(f"parent dir {d} group/other-writable")
print("; ".join(problems))
PY
)
if [ -n "$trust_verdict" ]; then
    warn "trust chain: $GUARD is user-replaceable ($trust_verdict)."
    warn "a managed config that executes a user-writable binary is not tamper-proof; install root-owned binaries first:"
    warn "  sudo BINDIR=/usr/local/bin make install"
    if [ "$FORCE" != "1" ] && [ "$DRY_RUN" != "1" ]; then
        err "refusing to install managed hooks over a user-writable guard (FORCE=1 to override for a trusted-user workstation)"
    fi
fi

# --- Optional PPG_URL pinning: wrap the guard in a root-owned launcher that
# fixes the validation server address, so the user environment cannot re-point content
# verification at a rogue validation server. Usage: PPG_PIN_URL=http://host:8765.
if [ -n "${PPG_PIN_URL:-}" ]; then
    WRAPPER="$(dirname "$TARGET")/ppg-guard-pinned.sh"
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would write root-owned wrapper $WRAPPER pinning PPG_URL=$PPG_PIN_URL"
    else
        mkdir -p "$(dirname "$WRAPPER")"
        cat > "$WRAPPER" <<WRAP
#!/bin/sh
# Root-owned launcher written by setup-claude-code-managed.sh: pins the
# validation server address so the user environment cannot re-point verification.
PPG_URL='$PPG_PIN_URL' export PPG_URL
exec '$GUARD' "\$@"
WRAP
        chmod 0755 "$WRAPPER"
        log "wrote pinning wrapper $WRAPPER (PPG_URL=$PPG_PIN_URL)"
    fi
    GUARD="$WRAPPER"
fi

python3 - "$TARGET" "$GUARD" <<PY
$(emit_pyhelpers)
file, guard_path = sys.argv[1:3]
dry   = os.environ.get('DRY_RUN') == '1'
force = os.environ.get('FORCE')   == '1'

try:
    data = json.load(open(file))
except FileNotFoundError:
    data = {}
except json.JSONDecodeError as e:
    print(f'[ppg] {file}: existing file is not valid JSON ({e}); refusing to overwrite', file=sys.stderr)
    sys.exit(1)

changed = []

# --- 1) allowManagedHooksOnly: true (the whole point of managed scope) ---
current = data.get('allowManagedHooksOnly')
if current is True:
    pass
elif current is None:
    data['allowManagedHooksOnly'] = True
    changed.append('allowManagedHooksOnly=true')
elif force:
    data['allowManagedHooksOnly'] = True
    changed.append(f'allowManagedHooksOnly: {json.dumps(current)} -> true (FORCE)')
else:
    print(f'[ppg] allowManagedHooksOnly is currently {json.dumps(current)}, not true; pass FORCE=1 to flip it')
    print(f'      leaving it as-is — ppg hooks below would still merge, but user/project hooks stay enabled')

# --- 2) Hook entries (surgical merge, same shape as user-scope) ----------
hooks = data.setdefault('hooks', {})

def has_guard(entries, matcher=None):
    for e in entries:
        if matcher is not None and e.get('matcher') != matcher:
            continue
        for h in e.get('hooks', []):
            if 'ppg-guard' in h.get('command', ''):
                return True
    return False

ss_entry = {'hooks': [{'type':'command','command':guard_path,'args':[]}]}
pt_entry = {'matcher':'Edit|Write','hooks':[{'type':'command','command':guard_path,'args':[]}]}

ss = hooks.setdefault('SessionStart', [])
if not has_guard(ss):
    ss.append(ss_entry)
    changed.append('hooks.SessionStart')

pt = hooks.setdefault('PreToolUse', [])
if not has_guard(pt, matcher='Edit|Write'):
    pt.append(pt_entry)
    changed.append('hooks.PreToolUse[Edit|Write]')

if not changed:
    print(f'[ppg] {file} already up-to-date, skipping')
    sys.exit(0)

if dry:
    print(f'[ppg] DRY_RUN: would write {file} (changes: {", ".join(changed)}):')
    print(json.dumps(data, indent=2))
    sys.exit(0)

backup_ts(file)
write_json_644(file, data)  # root-write, world-read — Claude Code reads as the user
print(f'[ppg] wrote {file}  (changes: {", ".join(changed)})')
PY

# --- 3) Cross-scope redundancy warning ------------------------------------
# Under allowManagedHooksOnly=true, any ppg hooks in the invoking user's
# ~/.claude/settings.json are silently ignored by Claude Code — flag them
# so the operator can clean up. Read the invoking user's HOME, not root's.
INVOKER=${SUDO_USER:-$USER}
if [ "$INVOKER" != root ] && [ "${DRY_RUN:-0}" != "1" ]; then
    if command -v getent >/dev/null 2>&1; then
        USER_HOME=$(getent passwd "$INVOKER" | cut -d: -f6 || true)
    else
        USER_HOME=$(sudo -u "$INVOKER" -H bash -c 'echo $HOME' 2>/dev/null || true)
    fi
    if [ -n "${USER_HOME:-}" ] && [ -f "$USER_HOME/.claude/settings.json" ]; then
        if grep -q 'ppg-guard' "$USER_HOME/.claude/settings.json"; then
            warn "user-scope hooks remain at $USER_HOME/.claude/settings.json — redundant under allowManagedHooksOnly."
            warn "run 'make remove-claude-code' as $INVOKER to clean them up."
        fi
    fi
fi

ok "Claude Code managed-scope setup complete."
log "Next: verify with 'claude' — SessionStart should fire from the managed file even in a fresh project."
