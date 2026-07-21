#!/usr/bin/env bash
#
# service-catalog-demo.sh — end-to-end proof of the PPG Service Catalog.
#
# Companion to docs/tutorials/13-discover-a-platform-service.md. It shows the
# discovery + enforcement loop:
#   - POST /discover_service ranks the catalog and returns the sanctioned
#     service (endpoint + API usage), excluding deprecated/forbidden ones;
#   - the recommended endpoint actually answers (a local svc-mock);
#   - ppg-guard refuses code that reaches for a forbidden provider (ADR-110)
#     and passes code that calls the sanctioned gateway.
#
# HERMETIC: its own validation server on a free port (your :8765 stays up), its own
# svc-mock, its own PPG_STORE_ROOT and temp git project — all removed on exit.
# It drives the INSTALLED binaries, so a green run proves the real behavior.
#
# Usage: bash scripts/service-catalog-demo.sh   (from the repo root)
# Exit:  0 if every case behaved as documented, non-zero otherwise.

set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
for bin in ppg ppg-guard svc-mock curl python3 git; do
  command -v "$bin" >/dev/null 2>&1 || {
    echo "service-catalog-demo: '$bin' not on PATH. Run 'make install' first." >&2
    exit 3
  }
done

TMP="$(mktemp -d)"
PROJ="$(cd "$TMP" && mkdir -p proj && cd proj && pwd -P)"
STATE="$TMP/state"; mkdir -p "$STATE"
GW_PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')"
MOCK_PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')"
GW="http://127.0.0.1:$GW_PORT"
MOCK="http://127.0.0.1:$MOCK_PORT"
export PPG_STORE_ROOT="$STATE" PPG_PROJECT_DIR="$PROJ"

cleanup() {
  [ -n "${GW_PID:-}" ] && kill "$GW_PID" >/dev/null 2>&1
  [ -n "${MOCK_PID:-}" ] && kill "$MOCK_PID" >/dev/null 2>&1
  rm -rf "$TMP"
}
trap cleanup EXIT

ppg -addr "127.0.0.1:$GW_PORT" \
  -adr "$REPO/examples/adr" -services "$REPO/examples/services" \
  -service-policy "$REPO/examples/service-policy" -skill-governance "$REPO/skill-governance" \
  >"$TMP/gateway.log" 2>&1 &
GW_PID=$!
svc-mock -addr "127.0.0.1:$MOCK_PORT" -name notify-svc >"$TMP/mock.log" 2>&1 &
MOCK_PID=$!
for _ in $(seq 1 50); do curl -sf "$GW/debt_report" >/dev/null 2>&1 && break; sleep 0.1; done
curl -sf "$GW/debt_report" >/dev/null 2>&1 || { echo "validation server never came up"; cat "$TMP/gateway.log"; exit 3; }
for _ in $(seq 1 50); do curl -sf "$MOCK/healthz" >/dev/null 2>&1 && break; sleep 0.1; done

pass=0; fail=0
check() { # desc  actual  expected
  if [ "$2" = "$3" ]; then printf '  \033[32mPASS\033[0m  %s\n' "$1"; pass=$((pass+1))
  else printf '  \033[31mFAIL\033[0m  %s\n        got:  %s\n        want: %s\n' "$1" "$2" "$3"; fail=$((fail+1)); fi
}
checkc() { # desc  haystack  needle
  if printf '%s' "$2" | grep -qF -- "$3"; then printf '  \033[32mPASS\033[0m  %s\n' "$1"; pass=$((pass+1))
  else printf '  \033[31mFAIL\033[0m  %s\n        looked for: %s\n        in: %s\n' "$1" "$3" "$2"; fail=$((fail+1)); fi
}

# jqp CAPABILITY-JSON PYEXPR -> evaluates a python expr over the parsed response `d`
disc() { curl -s -H 'content-type: application/json' -d "$1" "$GW/discover_service"; }
pyget() { printf '%s' "$1" | python3 -c "import sys,json; d=json.load(sys.stdin); print($2)"; }

echo
echo "Service Catalog demo — validation server $GW, notify-svc mock $MOCK"
echo

echo "── discovery: policy-ranked recommendation ──"
NOTIF="$(disc '{"capability":"notification"}')"
check "notification → status SERVICE_FOUND" "$(pyget "$NOTIF" 'd["status"]')" "SERVICE_FOUND"
check "notification → recommended notify-svc" "$(pyget "$NOTIF" 'd["recommended"]["service_id"]')" "notify-svc"
check "recommendation carries an endpoint" "$(pyget "$NOTIF" 'bool(d["recommended"]["endpoint"])')" "True"
check "recommendation carries API usage" "$(pyget "$NOTIF" 'bool(d["recommended"]["api_usage"])')" "True"
check "legacy-mailer surfaced as deprecated alternative" \
  "$(pyget "$NOTIF" 'next((a["status"] for a in d["alternatives"] if a["service_id"]=="legacy-mailer"),"missing")')" "deprecated"

# Intent-driven discovery resolves the capability from the wording.
PAYI="$(disc '{"intent":"let users pay by card at checkout"}')"
check "payment (by intent) → recommended payments-gateway" "$(pyget "$PAYI" 'd["recommended"]["service_id"]')" "payments-gateway"

# Capability-driven discovery returns the whole candidate set, including the
# forbidden sibling, so the caller sees what NOT to use and why.
PAY="$(disc '{"capability":"payment"}')"
check "payment → recommended payments-gateway" "$(pyget "$PAY" 'd["recommended"]["service_id"]')" "payments-gateway"
check "stripe-direct denied (forbidden)" \
  "$(pyget "$PAY" 'next((a["status"] for a in d["alternatives"] if a["service_id"]=="stripe-direct"),"missing")')" "forbidden"
checkc "stripe-direct denial carries a policy reason" "$PAY" "forbidden by platform policy"

echo
echo "── reachability: the recommended endpoint actually answers ──"
MOCK_CODE="$(curl -s -o /dev/null -w '%{http_code}' -H 'content-type: application/json' \
  -d '{"channel":"email","to":"a@b.c","template":"welcome"}' "$MOCK/v1/messages")"
check "notify-svc mock POST /v1/messages → 202" "$MOCK_CODE" "202"

echo
echo "── enforcement: ADR-110 at write time (ppg-guard) ──"
mkdir -p "$PROJ/internal/pay"
printf 'package pay\n' >"$PROJ/internal/pay/client.go"
( cd "$PROJ" && git init -q && git -c user.email=t@t -c user.name=t add -A && git -c user.email=t@t -c user.name=t commit -qm base )
printf '{"hook_event_name":"SessionStart","session_id":"S1","cwd":"%s"}' "$PROJ" \
  | ppg-guard --store-root "$STATE" --project-dir "$PROJ" >/dev/null 2>&1
PLAN='{"session_id":"S1","intent":"integrate the payments capability","repository_context":{"name":"checkout","tech_stack":["Go"]},"steps":[{"id":"s1","action":"add payment client","tool":"patch_code","targets":["internal/pay"]},{"id":"s2","action":"go test ./...","tool":"go-test","targets":["internal/pay"]}]}'
TICKET="$(curl -s -H 'content-type: application/json' -d "$PLAN" "$GW/lock_in_plan" | python3 -c 'import sys,json;print(json.load(sys.stdin).get("execution_ticket",""))')"
D="$(ls -d "$STATE"/projects/*/)"; mkdir -p "${D}tickets"; printf '%s' "$TICKET" >"${D}tickets/S1"

pretool() { python3 -c 'import json,sys;print(json.dumps({"hook_event_name":"PreToolUse","tool_name":"Write","session_id":sys.argv[1],"cwd":sys.argv[4],"tool_input":{"file_path":sys.argv[2],"content":sys.argv[3]}}))' "$1" "$2" "$3" "$PROJ"; }
guard() { G_OUT="$(printf '%s' "$1" | PPG_URL="$GW" ppg-guard --store-root "$STATE" --project-dir "$PROJ" 2>&1)"; G_RC=$?; }

guard "$(pretool S1 "$PROJ/internal/pay/client.go" 'package pay
import stripe "github.com/stripe/stripe-go/v76"
var _ = stripe.Key')"
check "forbidden Stripe SDK import → guard blocks (rc 2)" "$G_RC" "2"
checkc "block cites ADR-110 invariant" "$G_OUT" "ARCHITECTURAL_INVARIANT_VIOLATION"

guard "$(pretool S1 "$PROJ/internal/pay/client.go" 'package pay
const gateway = "http://localhost:9120/v1/charges"')"
check "sanctioned gateway call → guard passes (rc 0)" "$G_RC" "0"

echo
printf 'Result: %d passed, %d failed.\n' "$pass" "$fail"
[ "$fail" -eq 0 ] || exit 1
