#!/usr/bin/env bash
# Undo setup-github-copilot: remove the ppg MCP entry from
# ~/.copilot/mcp-config.json and delete the ppg-dedicated hook file
# ~/.copilot/hooks/ppg.json.
#
# Env: DRY_RUN=1 (preview)

set -euo pipefail
source "$(dirname "$0")/lib.sh"

MCP_FILE="$HOME/.copilot/mcp-config.json"
HOOK_FILE="$HOME/.copilot/hooks/ppg.json"

# --- 1) Remove mcpServers.ppg from ~/.copilot/mcp-config.json --------------
if [ -f "$MCP_FILE" ]; then
    python3 - "$MCP_FILE" <<PY
$(emit_pyhelpers)
file = sys.argv[1]
dry = os.environ.get('DRY_RUN') == '1'

try:
    data = json.load(open(file))
except json.JSONDecodeError as e:
    print(f'[ppg] {file}: existing file is not valid JSON ({e}); refusing to touch', file=sys.stderr)
    sys.exit(1)

mcp = data.get('mcpServers', {})
if 'ppg' not in mcp:
    print(f'[ppg] no mcpServers.ppg in {file}, skipping')
    sys.exit(0)

removed = mcp.pop('ppg')
if not mcp:
    del data['mcpServers']

if dry:
    print(f'[ppg] DRY_RUN: would remove mcpServers.ppg from {file}:')
    print(json.dumps({'mcpServers': {'ppg': removed}}, indent=2))
    sys.exit(0)

backup_ts(file)
write_json_600(file, data)
print(f'[ppg] wrote {file}  (removed mcpServers.ppg)')
PY
else
    log "no $MCP_FILE — nothing to remove there"
fi

# --- 2) Delete the ppg-dedicated hook file --------------------------------
if [ -f "$HOOK_FILE" ]; then
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would remove $HOOK_FILE (backup would be taken first)"
    else
        backup "$HOOK_FILE"
        rm -f "$HOOK_FILE"
        log "removed $HOOK_FILE"
    fi
else
    log "no $HOOK_FILE — nothing to remove"
fi

# --- 3) Detect the current project's ppg registrations (warn only) ---------
# The adapter README / tutorial 7 have users place ppg config INSIDE their
# project repos (often git-committed): a guard hook in .github/hooks/ppg.json
# or .claude/settings.json, and an MCP entry in .vscode/mcp.json. Those can't
# be safely rewritten from here, so report each with the exact removal step.
# Only the CURRENT project is inspected — other projects are never touched.
python3 - "$PWD" <<PY
$(emit_pyhelpers)
folder = sys.argv[1]

found = []

hook = os.path.join(folder, '.github', 'hooks', 'ppg.json')
if os.path.isfile(hook):
    found.append((hook, f'delete {hook}'))

settings = os.path.join(folder, '.claude', 'settings.json')
try:
    data = json.load(open(settings))
    if 'ppg' in json.dumps(data.get('hooks', {})):
        found.append((settings, f'remove the ppg-guard hook entries from {settings}'))
except (FileNotFoundError, json.JSONDecodeError, OSError):
    pass

vscode = os.path.join(folder, '.vscode', 'mcp.json')
try:
    data = json.load(open(vscode))
    servers = data.get('servers', {})
    if isinstance(servers, dict) and 'ppg' in servers:
        found.append((vscode, f'remove the "ppg" key from servers in {vscode}'))
except (FileNotFoundError, json.JSONDecodeError, OSError):
    pass

if found:
    print('[ppg] note: this project still has ppg registrations (edit these yourself — they may be git-committed):')
    for path, how in found:
        print(f'         {path}')
        print(f'             -> {how}')
PY

ok "GitHub Copilot teardown complete."
log "State directory preserved. Wipe manually with: rm -rf \"\${XDG_STATE_HOME:-\$HOME/.local/state}/ppg\""
