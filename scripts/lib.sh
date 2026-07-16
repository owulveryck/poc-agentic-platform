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
def write_json_600(file, data):
    os.makedirs(os.path.dirname(file) or '.', exist_ok=True)
    tmp = file + '.tmp.new'
    with open(tmp, 'w') as f:
        json.dump(data, f, indent=2)
    os.chmod(tmp, 0o600)
    os.replace(tmp, file)
PYHELPERS
}
