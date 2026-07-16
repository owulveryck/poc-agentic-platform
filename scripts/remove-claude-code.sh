#!/usr/bin/env bash
# Undo setup-claude-code: remove ppg MCP registrations and ppg-guard hook
# entries. Scope is limited to global (user-scope) config and the CURRENT
# project — other projects on this machine are never touched. Non-ppg MCP
# servers and non-ppg hooks are always preserved.
#
# Env: DRY_RUN=1 (preview)

set -euo pipefail
source "$(dirname "$0")/lib.sh"

MCP_FILE="$HOME/.claude.json"
HOOKS_FILE="$HOME/.claude/settings.json"

# --- 1) Remove ppg registrations from ~/.claude.json -----------------------
# Scope: global (user-scope) config + the CURRENT project only — never other
# projects on this machine. Removes the top-level mcpServers.ppg (user scope)
# and, for the current project, projects.<cwd>.mcpServers.ppg (local scope)
# plus the stale "ppg" entry in its enabled/disabledMcpjsonServers approval
# lists. This all lives in personal home-scope config, safe to rewrite.
if [ -f "$MCP_FILE" ]; then
    python3 - "$MCP_FILE" "$PWD" <<PY
$(emit_pyhelpers)
file, cwd = sys.argv[1:3]
dry = os.environ.get('DRY_RUN') == '1'

try:
    data = json.load(open(file))
except json.JSONDecodeError as e:
    print(f'[ppg] {file}: existing file is not valid JSON ({e}); refusing to touch', file=sys.stderr)
    sys.exit(1)

def same_dir(a, b):
    if a == b:
        return True
    try:
        return os.path.realpath(a) == os.path.realpath(b)
    except OSError:
        return False

changes = []

# Top-level (user-scope) mcpServers.ppg.
mcp = data.get('mcpServers', {})
if isinstance(mcp, dict) and 'ppg' in mcp:
    mcp.pop('ppg')
    if not mcp:
        data.pop('mcpServers', None)
    changes.append('mcpServers.ppg (top-level)')

# Current-project (local-scope) registration + stale approval state only.
for path, meta in data.get('projects', {}).items():
    if not isinstance(meta, dict) or not same_dir(path, cwd):
        continue
    pm = meta.get('mcpServers', {})
    if isinstance(pm, dict) and 'ppg' in pm:
        pm.pop('ppg')
        if not pm:
            meta.pop('mcpServers', None)
        changes.append(f'projects[{path}].mcpServers.ppg')
    for key in ('enabledMcpjsonServers', 'disabledMcpjsonServers'):
        lst = meta.get(key)
        if isinstance(lst, list) and 'ppg' in lst:
            meta[key] = [s for s in lst if s != 'ppg']
            changes.append(f'projects[{path}].{key}[ppg]')

if not changes:
    print(f'[ppg] no ppg registrations in {file}, skipping')
    sys.exit(0)

if dry:
    print(f'[ppg] DRY_RUN: would remove {len(changes)} ppg registration(s) from {file}:')
    for c in changes:
        print(f'         {c}')
    sys.exit(0)

backup_ts(file)
write_json_600(file, data)
print(f'[ppg] wrote {file}  (removed {len(changes)} ppg registration(s))')
for c in changes:
    print(f'         {c}')
PY
else
    log "no $MCP_FILE — nothing to remove there"
fi

# --- 1b) Remove ppg from ~/.mcp.json; warn on the current project's .mcp.json
# A project-scoped .mcp.json with an unapproved ppg server is what makes
# Claude Code prompt "New MCP server found, use it?" on every launch. The
# home-scope ~/.mcp.json is safe to rewrite; a .mcp.json inside the current
# project repo is frequently git-committed, so it is reported, not edited.
# Other projects on this machine are never inspected.
python3 - "$HOME/.mcp.json" "$PWD" <<PY
$(emit_pyhelpers)
home_mcp, cwd = sys.argv[1:3]
dry = os.environ.get('DRY_RUN') == '1'

def strip_ppg(path, auto):
    try:
        data = json.load(open(path))
    except FileNotFoundError:
        return
    except (json.JSONDecodeError, OSError) as e:
        print(f'[ppg] {path}: not readable JSON ({e}); skipping', file=sys.stderr)
        return
    mcp = data.get('mcpServers', {})
    if not isinstance(mcp, dict) or 'ppg' not in mcp:
        return
    if not auto:
        print(f'[ppg] note: the current project .mcp.json still registers ppg:')
        print(f'         {path}')
        print(f'      remove the "ppg" key from its mcpServers (it may be git-committed).')
        return
    mcp.pop('ppg')
    only_ppg = not mcp and set(data.keys()) <= {'mcpServers'}
    if not mcp:
        data.pop('mcpServers', None)
    if dry:
        act = 'delete (contained only ppg)' if only_ppg else 'remove mcpServers.ppg'
        print(f'[ppg] DRY_RUN: would {act} in {path}')
        return
    backup_ts(path)
    if only_ppg:
        os.remove(path)
        print(f'[ppg] removed {path}  (contained only ppg)')
    else:
        write_json_600(path, data)
        print(f'[ppg] wrote {path}  (removed mcpServers.ppg)')

# Home-scope personal config: safe to edit.
strip_ppg(home_mcp, auto=True)

# Current project only (repo-committed): warn, do not edit.
proj_mcp = os.path.join(cwd, '.mcp.json')
if proj_mcp != home_mcp:
    strip_ppg(proj_mcp, auto=False)
PY

# --- 2) Strip ppg-guard entries from ~/.claude/settings.json --------------
if [ -f "$HOOKS_FILE" ]; then
    python3 - "$HOOKS_FILE" <<PY
$(emit_pyhelpers)
file = sys.argv[1]
dry = os.environ.get('DRY_RUN') == '1'

try:
    data = json.load(open(file))
except json.JSONDecodeError as e:
    print(f'[ppg] {file}: existing file is not valid JSON ({e}); refusing to touch', file=sys.stderr)
    sys.exit(1)

hooks = data.get('hooks', {})
def is_ppg(h): return 'ppg-guard' in h.get('command', '')

removed = 0
removed_detail = {}
for event, entries in list(hooks.items()):
    new_entries = []
    for e in entries:
        e_hooks = e.get('hooks', [])
        keep    = [h for h in e_hooks if not is_ppg(h)]
        gone    = [h for h in e_hooks if is_ppg(h)]
        if gone:
            removed += len(gone)
            removed_detail.setdefault(event, []).extend(gone)
        if keep:
            e['hooks'] = keep
            new_entries.append(e)
        # else drop the wrapper (no non-ppg hooks left in it)
    if new_entries:
        hooks[event] = new_entries
    else:
        del hooks[event]

if not hooks:
    data.pop('hooks', None)

if removed == 0:
    print(f'[ppg] no ppg-guard hook entries in {file}, skipping')
    sys.exit(0)

if dry:
    print(f'[ppg] DRY_RUN: would remove {removed} ppg-guard hook entries from {file}:')
    print(json.dumps({'hooks': removed_detail}, indent=2))
    sys.exit(0)

backup_ts(file)
write_json_600(file, data)
print(f'[ppg] wrote {file}  (removed {removed} ppg-guard hook entries)')
PY
else
    log "no $HOOKS_FILE — nothing to remove there"
fi

ok "Claude Code teardown complete."
log "State directory preserved. Wipe manually with: rm -rf \"\${XDG_STATE_HOME:-\$HOME/.local/state}/ppg\""
