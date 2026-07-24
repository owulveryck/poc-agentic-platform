#!/usr/bin/env bash
#
# Deterministic proof that BMAD's structural directives are ENFORCED by ppg.
#
# BMAD (like any spec-driven method) hands the agent a process to follow: read
# the story, keep every story's Acceptance Criteria, stay inside the story's
# scope. Those are advisory — a model can skip them and simply *claim* it did.
# This driver runs, for each such directive, the two worlds:
#
#   * WITHOUT the harness: producing the offending artifact is a plain file
#     write. Nothing looks at it; the drift ships silently.
#   * WITH the harness: the exact same bytes hit /verify_artifact (content) or
#     /lock_in_plan (plan), or fall outside the capability ticket, and ppg
#     rejects them deterministically — the same calls the ppg-guard hook makes
#     on the agent's behalf inside Claude Code.
#
# It runs entirely at the HTTP level against a throwaway server on the BMAD ADR
# corpus (demo/bmad/adr). It mutates NO global config and needs no live agent.
#
# Usage:  bash demo/bmad/run-bmad-tests.sh
# Exit:   0 if every assertion held, 1 otherwise.

set -u

# --- locate repo root (this script lives in demo/bmad) -----------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FIX="$SCRIPT_DIR/fixtures"
PAY="$SCRIPT_DIR/payloads"
ADR_DIR="$SCRIPT_DIR/adr"

PORT="${PPG_TEST_PORT:-8811}"
BASE="http://127.0.0.1:$PORT"
# Sign tickets with a fresh, ephemeral secret each run (never committed). The
# throwaway server and the verify calls share this process env, so a random
# value is all that is needed — and it keeps the driver itself clean of any
# hardcoded credential.
export PPG_TICKET_SECRET="${PPG_TICKET_SECRET:-$(uuidgen)}"

# BMAD artifact paths the demo revolves around.
STORY_PATH="_bmad-output/implementation-artifacts/1-2-stripe-checkout.md"
IMPL_PATH="src/checkout/service.py"
OOS_PATH="src/auth/login.py"

# The KPI a presenter greps for: an Acceptance Criteria *section heading*, which
# is exactly what ADR-210 requires. Matching the heading (not a stray mention of
# the phrase in prose or a comment) keeps the "0 vs 1" contrast honest.
AC_HEADING='^##[[:space:]]+Acceptance Criteria'

PASS=0
FAIL=0
TMP="$(mktemp -d)"
trap 'cleanup' EXIT

cleanup() {
	[ -n "${SRV_PID:-}" ] && kill "$SRV_PID" 2>/dev/null
	rm -rf "$TMP"
}

say()  { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok()   { PASS=$((PASS+1)); printf '  \033[32mPASS\033[0m %s\n' "$*"; }
bad()  { FAIL=$((FAIL+1)); printf '  \033[31mFAIL\033[0m %s\n' "$*"; }

# check <description> <expected_http> <actual_http> [response_body_file]
check() {
	if [ "$2" = "$3" ]; then
		ok "$1 (HTTP $3)"
	else
		bad "$1 — expected HTTP $2, got $3"
		[ -n "${4:-}" ] && [ -s "$4" ] && sed 's/^/       /' "$4"
	fi
}

# post <endpoint> <json-body-or-@file> -> sets HTTP in $CODE, body in $TMP/resp
post() {
	local ep="$1" data="$2"
	CODE=$(curl -s -o "$TMP/resp" -w '%{http_code}' \
		-X POST "$BASE$ep" -H 'content-type: application/json' \
		--data "$data")
}

# verify_artifact <fixture-file> <artifact-path> -> $CODE / $TMP/resp
verify_artifact() {
	local body
	body=$(jq -n --arg t "$TICKET" --arg p "$2" --rawfile c "$1" \
		'{ticket:$t, path:$p, content:$c, op:"write"}')
	post /verify_artifact "$body"
}

# --- build & start the validation server on the BMAD corpus ------------------
say "Building ppg and starting it on the BMAD ADR corpus ($ADR_DIR)"
if ! go build -o "$TMP/ppg" "$REPO_ROOT/cmd/ppg" 2>"$TMP/build.log"; then
	echo "go build failed:"; cat "$TMP/build.log"; exit 1
fi
"$TMP/ppg" -adr "$ADR_DIR" -addr "127.0.0.1:$PORT" >"$TMP/server.log" 2>&1 &
SRV_PID=$!

for _ in $(seq 1 40); do
	if curl -s -m 1 "$BASE/debt_report" >/dev/null 2>&1; then break; fi
	sleep 0.25
done
if ! curl -s -m 2 "$BASE/debt_report" >/dev/null 2>&1; then
	echo "server did not start — log:"; cat "$TMP/server.log"; exit 1
fi
grep -E "ADR store loaded|Plan linter ready" "$TMP/server.log" | sed 's/^/  /'

# ---------------------------------------------------------------------------
# 0. Advisory layer: /enrich surfaces the invariants (guides, never blocks)
# ---------------------------------------------------------------------------
say "0. /enrich — the advisory layer (returns ADR text, does NOT enforce)"
post /enrich "@$PAY/enrich-story.json"
if [ "$CODE" = 200 ] && grep -qE "ADR-21[01]" "$TMP/resp"; then
	ok "a 'create a BMAD story' intent surfaces the BMAD invariants as context"
else
	bad "expected 200 mentioning ADR-210/ADR-211, got HTTP $CODE"; sed 's/^/       /' "$TMP/resp"
fi

# ---------------------------------------------------------------------------
# Obtain a capability ticket. Scope covers the story artifact and the in-scope
# implementation file only — this is the "story-scoped plan" a BMAD Dev locks
# before touching code. Built inline with Write steps so both paths are
# writable; a fresh session_id each run keeps anti-thrash out of the way.
# ---------------------------------------------------------------------------
say "Locking a story-scoped plan to obtain a capability ticket"
TICKET_PLAN=$(jq -n --arg s "$(uuidgen)" --arg sp "$STORY_PATH" --arg ip "$IMPL_PATH" \
	'{session_id:$s,
	  intent:"Author story 1.2 and implement it inside its scope",
	  repository_context:{name:"checkout-service", tech_stack:["Python"]},
	  steps:[
	    {id:"s1", action:"write the story", tool:"Write", targets:[$sp]},
	    {id:"s2", action:"implement the charge service against the story", tool:"Write", targets:[$ip]}
	  ]}')
post /lock_in_plan "$TICKET_PLAN"
check "story-scoped plan locks" 200 "$CODE" "$TMP/resp"
TICKET=$(jq -r '.execution_ticket // empty' "$TMP/resp")
[ -n "$TICKET" ] && ok "capability ticket issued (scope: story + $IMPL_PATH)" || bad "no ticket issued"

# ---------------------------------------------------------------------------
# 1. ADR-210 — every story keeps Acceptance Criteria + Tasks (artifact)
# ---------------------------------------------------------------------------
say "1. ADR-210 — story schema completeness (WITHOUT vs WITH harness)"

echo "  [WITHOUT harness] a bare copy of the AC-less story just... works:"
cp "$FIX/story.no-ac.md" "$TMP/story.md" && echo "       wrote $TMP/story.md — nothing checked it. Drift ships silently."
echo "       grep -c '## Acceptance Criteria' -> $(grep -cE "$AC_HEADING" "$TMP/story.md")  (the KPI: 0 = missing)"

echo "  [WITH harness] the same story sent to /verify_artifact:"
verify_artifact "$FIX/story.no-ac.md" "$STORY_PATH"
check "story with no Acceptance Criteria / Tasks is rejected" 422 "$CODE" "$TMP/resp"
verify_artifact "$FIX/story.good.md"  "$STORY_PATH"
check "complete BMAD story is accepted"                       200 "$CODE" "$TMP/resp"

# ---------------------------------------------------------------------------
# 2. ADR-211 — a plan that writes code must read the story first (plan)
# ---------------------------------------------------------------------------
say "2. ADR-211 — plan references the story (plan altitude, at /lock_in_plan)"
# Fresh session_id + intent each run so anti-thrash escalation (3 identical
# rejections -> 409 POLICY_CONFLICT) never accumulates; the ADR-211 violation
# still fires on the plan steps (422).
SKIP_PLAN=$(jq --arg s "$(uuidgen)" --arg i "Implement the Stripe checkout feature ($(uuidgen))" \
	'.session_id = $s | .intent = $i' "$PAY/plan-skip-story.json")
post /lock_in_plan "$SKIP_PLAN"
check "plan that writes src/ without reading the story is rejected" 422 "$CODE" "$TMP/resp"
GOOD_PLAN=$(jq --arg s "$(uuidgen)" '.session_id = $s' "$PAY/plan-good.json")
post /lock_in_plan "$GOOD_PLAN"
check "plan that reads the story before coding locks"              200 "$CODE" "$TMP/resp"

# ---------------------------------------------------------------------------
# 3. Ticket scope + authentication (the guard's first two gates)
# ---------------------------------------------------------------------------
say "3. Story-scope confinement and authentication"

echo "  [WITHOUT harness] a bare copy of the out-of-scope edit just... works:"
cp "$FIX/out-of-scope.py" "$TMP/login.py" && echo "       wrote $TMP/login.py ($OOS_PATH) — scope-creep ships silently."

echo "  [WITH harness] the in-scope implementation is allowed:"
verify_artifact "$FIX/impl.py" "$IMPL_PATH"
check "in-scope implementation ($IMPL_PATH) is accepted"     200 "$CODE" "$TMP/resp"
echo "  [WITH harness] the out-of-scope edit is refused:"
verify_artifact "$FIX/out-of-scope.py" "$OOS_PATH"
check "edit outside the story's ticket scope is refused"     403 "$CODE" "$TMP/resp"

INVALID_BODY=$(jq -n --rawfile c "$FIX/impl.py" \
	'{ticket:"not-a-real-ticket", path:"'"$IMPL_PATH"'", content:$c, op:"write"}')
post /verify_artifact "$INVALID_BODY"
check "no/invalid ticket is refused"                         401 "$CODE" "$TMP/resp"

# --- summary ---------------------------------------------------------------
say "Summary: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
