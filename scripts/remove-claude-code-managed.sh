#!/usr/bin/env bash
# Undo setup-claude-code-managed: strip ppg-guard hook entries from the
# managed-settings.json and drop allowManagedHooksOnly iff no non-ppg hooks
# remain. If the file ends up empty, remove it entirely. Non-ppg policy is
# always preserved.
#
# Requires root. Preview with DRY_RUN=1 (no sudo needed). Test path can be
# overridden with PPG_MANAGED_SETTINGS_PATH.
#
# Env: DRY_RUN=1 (preview, no root required), PPG_MANAGED_SETTINGS_PATH.

set -euo pipefail
source "$(dirname "$0")/lib.sh"

TARGET=$(managed_settings_path)
need_root

if [ ! -f "$TARGET" ]; then
    log "no $TARGET — nothing to remove"
    exit 0
fi

python3 - "$TARGET" <<PY
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

# Drop allowManagedHooksOnly iff no non-ppg hooks remain — we set it, and
# nobody else layered on top. If IT-authored hooks survive, preserve it.
dropped_allow = False
if not hooks:
    data.pop('hooks', None)
    if data.get('allowManagedHooksOnly') is True:
        data.pop('allowManagedHooksOnly')
        dropped_allow = True

if removed == 0 and not dropped_allow:
    print(f'[ppg] no ppg entries in {file}, skipping')
    sys.exit(0)

if dry:
    print(f'[ppg] DRY_RUN: would remove {removed} ppg-guard hook entries from {file}'
          + ('; would also drop allowManagedHooksOnly (no other policy remains)' if dropped_allow else ''))
    if removed_detail:
        print(json.dumps({'hooks': removed_detail}, indent=2))
    sys.exit(0)

backup_ts(file)

if not data:
    os.remove(file)
    print(f'[ppg] removed {file}  (contained only ppg entries)')
else:
    write_json_644(file, data)
    print(f'[ppg] wrote {file}  (removed {removed} ppg-guard hook entries'
          + (', dropped allowManagedHooksOnly' if dropped_allow else '') + ')')
PY

ok "Claude Code managed-scope teardown complete."
