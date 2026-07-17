#!/usr/bin/env bash
#
# quickstart.sh — a one-minute guided tour of the Platform Planning Gateway.
#
# Companion to the README "Quick start" (run it via `make quickstart`). It
# starts a throwaway gateway on the fictional demo corpus in examples/ and
# shows the three core moves:
#   1. /enrich          — retrieve the architectural invariants for an intent;
#   2. /lock_in_plan    — watch the deterministic linter reject a bad plan,
#                         then lock the fixed one (capability ticket);
#   3. /discover_service — ask the Service Catalog for the sanctioned service.
#
# HERMETIC: its own gateway on a free port, everything removed on exit. It
# drives the freshly BUILT binary ($REPO/bin/ppg), so it works right after
# `make build` — no `make install` needed.
#
# Usage: make quickstart      (or: bash scripts/quickstart.sh after make build)
# Exit:  0 if all three sections behaved as documented, non-zero otherwise.

set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
for bin in curl python3; do
  command -v "$bin" >/dev/null 2>&1 || {
    echo "quickstart: '$bin' not on PATH." >&2
    exit 3
  }
done
[ -x "$REPO/bin/ppg" ] || {
  echo "quickstart: $REPO/bin/ppg not found. Run 'make quickstart' (or 'make build' first)." >&2
  exit 3
}

TMP="$(mktemp -d)"
PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')"
GW="http://127.0.0.1:$PORT"

cleanup() {
  [ -n "${GW_PID:-}" ] && kill "$GW_PID" >/dev/null 2>&1
  rm -rf "$TMP"
}
trap cleanup EXIT

"$REPO/bin/ppg" -addr "127.0.0.1:$PORT" \
  -adr "$REPO/examples/adr" -services "$REPO/examples/services" \
  -service-policy "$REPO/examples/service-policy" -skill-governance "$REPO/skill-governance" \
  >"$TMP/gateway.log" 2>&1 &
GW_PID=$!
for _ in $(seq 1 50); do curl -sf "$GW/debt_report" >/dev/null 2>&1 && break; sleep 0.1; done
curl -sf "$GW/debt_report" >/dev/null 2>&1 || { echo "gateway never came up"; cat "$TMP/gateway.log"; exit 3; }

pass=0; fail=0
check() { # desc  actual  expected
  if [ "$2" = "$3" ]; then pass=$((pass+1))
  else printf '  \033[31mUNEXPECTED\033[0m  %s (got: %s, want: %s)\n' "$1" "$2" "$3"; fail=$((fail+1)); fi
}
pyget() { printf '%s' "$1" | python3 -c "import sys,json; d=json.load(sys.stdin); print($2)"; }
pretty() { printf '%s' "$1" | python3 -m json.tool; }
post() { curl -s -H 'content-type: application/json' -d "$2" "$GW$1"; }

echo
echo "PPG quickstart — throwaway gateway on $GW, demo corpus: examples/"
echo

echo "━━ 1. /enrich — which architectural invariants govern this intent? ━━"
echo
echo '   intent: "add an external payment provider"'
ENRICH="$(post /enrich '{"intent":"add an external payment provider","repository_context":{"name":"web","tech_stack":["Go"]}}')"
pretty "$ENRICH"
N_INV="$(pyget "$ENRICH" 'len(d.get("amplifier_context",{}).get("architectural_invariants",[]))')"
check "enrich returns invariants" "$(pyget "$ENRICH" 'str(len(d.get("amplifier_context",{}).get("architectural_invariants",[]))>0)')" "True"
echo
echo "   → $N_INV invariant(s) retrieved from the ADR store (ADR-042: external"
echo "     providers go through the platform proxy). The agent gets them BEFORE"
echo "     planning — governance as context, not as an after-the-fact block."
echo

echo "━━ 2. /lock_in_plan — the deterministic plan gate (OPA/Rego, not an LLM) ━━"
echo
echo "   First, a plan that touches Go code but has no test step:"
BAD_PLAN='{"session_id":"11111111-1111-1111-1111-111111111111","intent":"build a landing page","repository_context":{"name":"web","tech_stack":["Go"]},"steps":[{"id":"s1","action":"read design tokens","tool":"Read","targets":["design/tokens.css"]},{"id":"s2","action":"write styles","tool":"Write","targets":["index.css"]}]}'
REJECTED="$(post /lock_in_plan "$BAD_PLAN")"
pretty "$REJECTED"
check "bad plan is rejected" "$(pyget "$REJECTED" 'd["status"]')" "PLAN_REJECTED"
echo
echo "   Same plan with the go-test step added:"
GOOD_PLAN='{"session_id":"11111111-1111-1111-1111-111111111111","intent":"build a landing page","repository_context":{"name":"web","tech_stack":["Go"]},"steps":[{"id":"s1","action":"read design tokens","tool":"Read","targets":["design/tokens.css"]},{"id":"s2","action":"write styles","tool":"Write","targets":["index.css"]},{"id":"s3","action":"go test","tool":"go-test","targets":["x_test.go"]}]}'
LOCKED="$(post /lock_in_plan "$GOOD_PLAN")"
STATUS="$(pyget "$LOCKED" 'd["status"]')"
TICKET="$(pyget "$LOCKED" 'd.get("execution_ticket","")')"
check "fixed plan is locked" "$STATUS" "PLAN_LOCKED"
echo "   status: $STATUS"
echo "   execution_ticket: ${TICKET:0:40}…  (ephemeral JWT: plan hash + scope)"
echo
echo "   → Rejection came with semantic violations; the lock issued a"
echo "     least-privilege capability ticket the smart tools verify in-tool."
echo

echo "━━ 3. /discover_service — the sanctioned service for a capability ━━"
echo
echo '   capability: "notification"'
DISC="$(post /discover_service '{"capability":"notification"}')"
pretty "$DISC"
check "discovery finds a service" "$(pyget "$DISC" 'd["status"]')" "SERVICE_FOUND"
check "notify-svc is recommended" "$(pyget "$DISC" 'd["recommended"]["service_id"]')" "notify-svc"
echo
echo "   → notify-svc recommended (endpoint + verbatim API usage); the"
echo "     deprecated legacy-mailer is surfaced as a non-option, with the reason."
echo "     ADR-110 then makes the recommendation binding at write time."
echo

echo "━━ Next steps ━━"
echo
echo "  • Full planning cycle with curl:   docs/tutorials/01-first-planning-cycle.md"
echo "  • Discovery + write-time refusal:  docs/tutorials/13-discover-a-platform-service.md"
echo "                                     (bash scripts/service-catalog-demo.sh, needs make install)"
echo "  • Red-team every bypass trick:     docs/tutorials/12-bypassing-the-gateway.md"
echo "                                     (bash scripts/redteam-bypass.sh, needs make install)"
echo "  • Replace the fictional corpus with your own: examples/README.md"
echo

if [ "$fail" -ne 0 ]; then
  printf '\033[31m%d section(s) did not behave as documented.\033[0m\n' "$fail"
  exit 1
fi
