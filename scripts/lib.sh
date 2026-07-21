# Shared helpers for the setup / remove scripts.
# Source this from other scripts:  source "$(dirname "$0")/lib.sh"
#
# Env vars respected by callers:
#   DRY_RUN=1   print what would change; do not touch the filesystem
#   FORCE=1     overwrite a ppg entry that already exists but differs

set -euo pipefail

: "${DRY_RUN:=0}"
: "${FORCE:=0}"

# Export so the inline Python heredocs in the setup/remove scripts see them.
export DRY_RUN FORCE

log()  { printf '\033[36m[ppg]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[ppg]\033[0m %s\n' "$*" >&2; }
ok()   { printf '\033[32m[ppg]\033[0m %s\n' "$*"; }
err()  { printf '\033[31m[ppg]\033[0m %s\n' "$*" >&2; exit 1; }

# need_binary <name>
# Prints the absolute path (via `command -v`), or exits with a helpful message
# telling the user to `make install` first.
need_binary() {
    local name=$1 path
    if ! path=$(command -v "$name" 2>/dev/null); then
        err "'$name' not on PATH — run 'make install' first (or adjust BINDIR)"
    fi
    # Follow the resolved path to its real location for the config file, so
    # a symlink shuffle in ~/.local/bin does not silently redirect the hook.
    if command -v realpath >/dev/null 2>&1; then
        realpath "$path"
    else
        # BSD readlink fallback (macOS ships /usr/bin/readlink without -f).
        python3 -c "import os,sys; print(os.path.realpath(sys.argv[1]))" "$path"
    fi
}

# managed_settings_path
# Prints the OS-appropriate managed-settings.json path, or the override
# $PPG_MANAGED_SETTINGS_PATH when set (used by tests). Exits with err() on
# an unsupported OS. Reference: https://code.claude.com/docs/en/settings
managed_settings_path() {
    if [ -n "${PPG_MANAGED_SETTINGS_PATH:-}" ]; then
        printf '%s\n' "$PPG_MANAGED_SETTINGS_PATH"
        return
    fi
    case "$(uname -s)" in
        Darwin) printf '/Library/Application Support/ClaudeCode/managed-settings.json\n' ;;
        Linux)  printf '/etc/claude-code/managed-settings.json\n' ;;
        *) err "unsupported OS $(uname -s); on Windows install C:\\Program Files\\ClaudeCode\\managed-settings.json by hand (see docs/how-to/set-up-a-governed-workstation.md)" ;;
    esac
}

# need_root
# Exit 1 with a copy-pasteable sudo re-invocation if not running as root.
# DRY_RUN or a PPG_MANAGED_SETTINGS_PATH override skips the check — both are
# escape hatches for previewing and testing that don't touch the OS-level
# root-owned managed-settings path.
need_root() {
    [ "$DRY_RUN" = "1" ] && return 0
    [ -n "${PPG_MANAGED_SETTINGS_PATH:-}" ] && return 0
    if [ "$(id -u)" -ne 0 ]; then
        err "this script writes root-owned files; re-run as: sudo -E env PATH=\"$PATH\" $0"
    fi
}

# backup <file>
# Copies <file> to <file>.bak-YYYYMMDDHHMMSS if it exists. No-op if absent.
# In DRY_RUN mode, prints the planned backup without copying.
# Used by remove-* scripts where a write is always performed; setup-*
# scripts backup inside their Python (only when a write is actually needed).
backup() {
    local file=$1
    [ -e "$file" ] || return 0
    local ts backup_path
    ts=$(date +%Y%m%d%H%M%S)
    backup_path="${file}.bak-${ts}"
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would back up $file → $backup_path"
    else
        cp -p "$file" "$backup_path"
        log "backup: $backup_path"
    fi
}

# emit_pyhelpers
# Prints (to stdout) the Python `write_json_600` and `backup_ts` helpers
# for embedding into a heredoc. Use as:
#   python3 - args... <<PY
#   $(emit_pyhelpers)
#   ... your code that calls backup_ts(file) then write_json_600(file, data)
#   PY
emit_pyhelpers() {
    cat <<'PYHELPERS'
import json, os, shutil, sys, time
def backup_ts(file):
    if not os.path.exists(file): return None
    dst = f"{file}.bak-{time.strftime('%Y%m%d%H%M%S')}"
    shutil.copy2(file, dst)
    print(f"[ppg] backup: {dst}")
    return dst
def write_json_mode(file, data, mode):
    os.makedirs(os.path.dirname(file) or '.', exist_ok=True)
    tmp = file + '.tmp.new'
    with open(tmp, 'w') as f:
        json.dump(data, f, indent=2)
    os.chmod(tmp, mode)
    os.replace(tmp, file)
def write_json_600(file, data):
    write_json_mode(file, data, 0o600)
def write_json_644(file, data):
    write_json_mode(file, data, 0o644)
PYHELPERS
}
