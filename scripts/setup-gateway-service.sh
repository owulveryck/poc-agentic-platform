#!/usr/bin/env bash
# Run the validation server (ppg) as a user service so a governed
# workstation does not depend on a terminal staying open: launchd on
# macOS (LaunchAgent), systemd --user on Linux.
#
# Every argument is passed through to ppg verbatim. Defaults to
# '-addr 127.0.0.1:8765' (loopback — the API is unauthenticated; pass an
# explicit host only behind a trusted network or an auth proxy). Examples:
#
#   scripts/setup-gateway-service.sh -adr "$HOME/corpus/adr"
#   scripts/setup-gateway-service.sh -adr "$HOME/corpus/adr" \
#       -services "$HOME/corpus/services" -service-policy "$HOME/corpus/service-policy"
#   scripts/setup-gateway-service.sh -skills "$HOME/.claude/skills"   # skill-only, no ADRs
#
# Env: DRY_RUN=1 (preview), FORCE=1 (overwrite an existing unit).

set -euo pipefail
source "$(dirname "$0")/lib.sh"

PPG=$(need_binary ppg)
ARGS=("$@")
[ ${#ARGS[@]} -eq 0 ] && ARGS=(-addr 127.0.0.1:8765)

case "$(uname -s)" in
Darwin)
    LABEL="io.ppg.gateway"
    PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
    LOG="$HOME/Library/Logs/ppg.log"
    if [ -e "$PLIST" ] && [ "$FORCE" != "1" ] && [ "$DRY_RUN" != "1" ]; then
        err "$PLIST already exists — FORCE=1 to overwrite, or scripts/remove-gateway-service.sh first"
    fi
    args_xml=""
    for a in "${ARGS[@]}"; do
        args_xml="$args_xml        <string>$a</string>
"
    done
    unit=$(cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>$LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$PPG</string>
$args_xml    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>$LOG</string>
    <key>StandardErrorPath</key><string>$LOG</string>
</dict>
</plist>
EOF
)
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would write $PLIST:"
        printf '%s\n' "$unit"
        exit 0
    fi
    backup "$PLIST"
    mkdir -p "$(dirname "$PLIST")"
    printf '%s\n' "$unit" > "$PLIST"
    launchctl unload "$PLIST" 2>/dev/null || true
    launchctl load -w "$PLIST"
    ok "loaded $LABEL (logs: $LOG)"
    log "manage with: launchctl unload/load $PLIST"
    ;;
Linux)
    UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
    UNIT="$UNIT_DIR/ppg.service"
    if [ -e "$UNIT" ] && [ "$FORCE" != "1" ] && [ "$DRY_RUN" != "1" ]; then
        err "$UNIT already exists — FORCE=1 to overwrite, or scripts/remove-gateway-service.sh first"
    fi
    unit=$(cat <<EOF
[Unit]
Description=PPG validation server (deterministic governance harness)

[Service]
ExecStart=$PPG ${ARGS[*]}
Restart=on-failure

[Install]
WantedBy=default.target
EOF
)
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would write $UNIT and 'systemctl --user enable --now ppg':"
        printf '%s\n' "$unit"
        exit 0
    fi
    backup "$UNIT"
    mkdir -p "$UNIT_DIR"
    printf '%s\n' "$unit" > "$UNIT"
    systemctl --user daemon-reload
    systemctl --user enable --now ppg.service
    ok "enabled ppg.service (journalctl --user -u ppg to follow logs)"
    ;;
*)
    err "unsupported OS $(uname -s) — run ppg from a terminal or your own service manager"
    ;;
esac
