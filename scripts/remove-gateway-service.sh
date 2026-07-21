#!/usr/bin/env bash
# Remove the validation-server user service installed by
# setup-gateway-service.sh (launchd LaunchAgent on macOS, systemd --user
# unit on Linux).
#
# Env: DRY_RUN=1 (preview).

set -euo pipefail
source "$(dirname "$0")/lib.sh"

case "$(uname -s)" in
Darwin)
    PLIST="$HOME/Library/LaunchAgents/io.ppg.gateway.plist"
    if [ ! -e "$PLIST" ]; then
        log "no $PLIST — nothing to do"
        exit 0
    fi
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would unload and delete $PLIST"
        exit 0
    fi
    launchctl unload "$PLIST" 2>/dev/null || true
    rm -f "$PLIST"
    ok "unloaded and removed $PLIST"
    ;;
Linux)
    UNIT="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/ppg.service"
    if [ ! -e "$UNIT" ]; then
        log "no $UNIT — nothing to do"
        exit 0
    fi
    if [ "$DRY_RUN" = "1" ]; then
        log "DRY_RUN: would disable ppg.service and delete $UNIT"
        exit 0
    fi
    systemctl --user disable --now ppg.service 2>/dev/null || true
    rm -f "$UNIT"
    systemctl --user daemon-reload
    ok "disabled and removed ppg.service"
    ;;
*)
    err "unsupported OS $(uname -s)"
    ;;
esac
