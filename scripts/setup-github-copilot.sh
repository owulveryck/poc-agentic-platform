#!/usr/bin/env bash
# Register the ppg MCP server user-scope for GitHub Copilot and install the
# ppg-copilot-guard hook (dedicated ~/.copilot/hooks/ppg.json — safe to
# overwrite; a backup is taken first).
#
# Idempotent for MCP; the hook file is ppg-owned by convention so it is
# always (re)written to the canonical shape.
#
# Env: DRY_RUN=1 (preview), FORCE=1 (overwrite differing ppg MCP entry),
#      PPG_URL (default http://localhost:8765).

set -euo pipefail
source "$(dirname "$0")/lib.sh"

MCP=$(need_binary ppg-mcp-server)
GUARD=$(need_binary ppg-copilot-guard)
PPG_URL="${PPG_URL:-http://localhost:8765}"

MCP_FILE="$HOME/.copilot/mcp-config.json"
HOOK_FILE="$HOME/.copilot/hooks/ppg.json"

# --- 1) Register mcpServers.ppg in ~/.copilot/mcp-config.json --------------
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
    'tools':   ['*'],
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

# --- 2) Write dedicated hook file ~/.copilot/hooks/ppg.json ---------------
python3 - "$HOOK_FILE" "$GUARD" <<PY
$(emit_pyhelpers)
file, guard_path = sys.argv[1:3]
dry = os.environ.get('DRY_RUN') == '1'

target = {
    'hooks': {
        'SessionStart': [
            {'type': 'command', 'command': guard_path, 'timeoutSec': 5},
        ],
        'PreToolUse': [
            {'type': 'command', 'command': guard_path, 'timeoutSec': 5},
        ],
    }
}

try:
    current = json.load(open(file))
    if current == target:
        print(f'[ppg] {file} already up-to-date, skipping')
        sys.exit(0)
except (FileNotFoundError, json.JSONDecodeError):
    pass  # will be (re)written below

if dry:
    print(f'[ppg] DRY_RUN: would write {file}:')
    print(json.dumps(target, indent=2))
    sys.exit(0)

backup_ts(file)
write_json_600(file, target)
print(f'[ppg] wrote {file}  (ppg-dedicated hook file)')
PY

ok "GitHub Copilot setup complete."
log "Next: start the validation server ('ppg -addr 127.0.0.1:8765'). Verify with 'copilot mcp list' (if the CLI is installed), or open the Copilot app and check the tool drawer."
if ! curl -fsS --max-time 1 "$PPG_URL/enrich" -H 'content-type: application/json' -d '{"intent":"ping","repository_context":{"name":"x","tech_stack":["Go"]}}' >/dev/null 2>&1; then
    warn "validation server at $PPG_URL is not reachable — start it with 'ppg -addr 127.0.0.1:8765' before opening Copilot."
fi
