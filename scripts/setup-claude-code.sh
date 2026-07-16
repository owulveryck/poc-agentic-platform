#!/usr/bin/env bash
# Register the ppg MCP server user-scope and install the ppg-guard hooks
# (SessionStart + PreToolUse on Edit|Write) into ~/.claude/settings.json.
#
# Idempotent. Backs up any file it actually modifies. Never touches
# non-ppg entries.
#
# Env: DRY_RUN=1 (preview), FORCE=1 (overwrite differing ppg MCP entry),
#      PPG_URL (default http://localhost:8765).

set -euo pipefail
source "$(dirname "$0")/lib.sh"

MCP=$(need_binary ppg-mcp-server)
GUARD=$(need_binary ppg-guard)
PPG_URL="${PPG_URL:-http://localhost:8765}"

MCP_FILE="$HOME/.claude.json"
HOOKS_FILE="$HOME/.claude/settings.json"

# --- 1) Register the MCP server under top-level mcpServers.ppg -------------
python3 - "$MCP_FILE" "$MCP" "$PPG_URL" <<PY
$(emit_pyhelpers)
file, mcp_path, ppg_url = sys.argv[1:4]
dry   = os.environ.get('DRY_RUN') == '1'
force = os.environ.get('FORCE')   == '1'

try:
    data = json.load(open(file))
except FileNotFoundError:
    data = {}
except json.JSONDecodeError as e:
    print(f'[ppg] {file}: existing file is not valid JSON ({e}); refusing to overwrite', file=sys.stderr)
    sys.exit(1)

mcp = data.setdefault('mcpServers', {})
entry = {
    'type':    'stdio',
    'command': mcp_path,
    'args':    [],
    'env':     {'PPG_URL': ppg_url},
}

current = mcp.get('ppg')
if current == entry:
    print(f'[ppg] MCP entry already up-to-date in {file}, skipping')
    sys.exit(0)
if current is not None and not force:
    print(f'[ppg] MCP entry present but differs from target; pass FORCE=1 to overwrite')
    print(f'      current: {json.dumps(current)}')
    print(f'      target : {json.dumps(entry)}')
    sys.exit(0)

mcp['ppg'] = entry

if dry:
    action = 'overwrite' if current is not None else 'add'
    print(f'[ppg] DRY_RUN: would {action} mcpServers.ppg in {file}:')
    print(json.dumps({'mcpServers': {'ppg': entry}}, indent=2))
    sys.exit(0)

backup_ts(file)
write_json_600(file, data)
print(f'[ppg] wrote {file}  (mcpServers.ppg)')
PY

# --- 2) Merge hooks into ~/.claude/settings.json ---------------------------
python3 - "$HOOKS_FILE" "$GUARD" <<PY
$(emit_pyhelpers)
file, guard_path = sys.argv[1:3]
dry = os.environ.get('DRY_RUN') == '1'

try:
    data = json.load(open(file))
except FileNotFoundError:
    data = {}
except json.JSONDecodeError as e:
    print(f'[ppg] {file}: existing file is not valid JSON ({e}); refusing to overwrite', file=sys.stderr)
    sys.exit(1)

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

changed = []
added = {}
ss = hooks.setdefault('SessionStart', [])
if not has_guard(ss):
    ss.append(ss_entry)
    changed.append('SessionStart')
    added.setdefault('SessionStart', []).append(ss_entry)

pt = hooks.setdefault('PreToolUse', [])
if not has_guard(pt, matcher='Edit|Write'):
    pt.append(pt_entry)
    changed.append('PreToolUse[Edit|Write]')
    added.setdefault('PreToolUse', []).append(pt_entry)

if not changed:
    print(f'[ppg] hooks already present in {file}, skipping')
    sys.exit(0)

if dry:
    print(f'[ppg] DRY_RUN: would append to {file} (adding: {", ".join(changed)}):')
    print(json.dumps({'hooks': added}, indent=2))
    sys.exit(0)

backup_ts(file)
write_json_600(file, data)
print(f'[ppg] wrote {file}  (added: {", ".join(changed)})')
PY

ok "Claude Code setup complete."
log "Next: start the gateway ('ppg -addr :8765') and run 'claude mcp list' — you should see 'ppg  connected'."
if ! curl -fsS --max-time 1 "$PPG_URL/enrich" -H 'content-type: application/json' -d '{"intent":"ping","repository_context":{"name":"x","tech_stack":["Go"]}}' >/dev/null 2>&1; then
    warn "gateway at $PPG_URL is not reachable — start it with 'ppg -addr :8765' before opening claude."
fi
