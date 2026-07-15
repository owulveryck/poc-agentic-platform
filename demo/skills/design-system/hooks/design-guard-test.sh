#!/bin/bash
# design-guard-test.sh — fixture-based tests for design-guard.sh.
# Runs six payloads through the hook and asserts the decision + reason
# fragment. No test framework: pure shell + jq.

set -eu

HERE="$(cd "$(dirname "$0")" && pwd)"
GUARD="$HERE/design-guard.sh"

if [ ! -x "$GUARD" ]; then
  echo "test-setup: $GUARD is not executable — run 'chmod +x $GUARD' first." >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "test-setup: jq is required." >&2
  exit 1
fi

pass=0
fail=0

# assert_pass NAME PAYLOAD
assert_pass() {
  local name="$1" payload="$2"
  local out decision
  out=$(printf '%s' "$payload" | "$GUARD")
  decision=$(printf '%s' "$out" | jq -r '.continue // .hookSpecificOutput.permissionDecision // "?"')
  if [ "$decision" = "true" ]; then
    printf '  PASS  %s\n' "$name"
    pass=$((pass + 1))
  else
    printf '  FAIL  %s\n        got: %s\n' "$name" "$out"
    fail=$((fail + 1))
  fi
}

# assert_deny NAME EXPECTED_REASON_FRAGMENT PAYLOAD
assert_deny() {
  local name="$1" want="$2" payload="$3"
  local out decision reason
  out=$(printf '%s' "$payload" | "$GUARD")
  decision=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.permissionDecision // "?"')
  reason=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.permissionDecisionReason // ""')
  if [ "$decision" = "deny" ] && printf '%s' "$reason" | grep -qF "$want"; then
    printf '  PASS  %s\n' "$name"
    pass=$((pass + 1))
  else
    printf '  FAIL  %s\n        got decision=%s, reason=%q\n' "$name" "$decision" "$reason"
    fail=$((fail + 1))
  fi
}

echo "design-guard.sh fixture tests"

assert_deny \
  "raw hex color in style.css is denied" \
  "DESIGN_SYSTEM_VIOLATION" \
  '{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/proj","tool_input":{"path":"/proj/style.css","new_str":".hero{color:#F0F;}"}}'

assert_pass \
  "var(--color-primary) in style.css passes" \
  '{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/proj","tool_input":{"path":"/proj/style.css","new_str":".hero{color:var(--color-primary);}"}}'

assert_deny \
  "button rule outside tokens.css is denied" \
  "belongs in design/tokens.css only" \
  '{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/proj","tool_input":{"path":"/proj/style.css","new_str":"button{border-radius:var(--btn-radius);}"}}'

assert_pass \
  "button rule inside design/tokens.css passes" \
  '{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/proj","tool_input":{"path":"/proj/design/tokens.css","new_str":"button{color:var(--color-primary);}"}}'

assert_pass \
  "hex color inside design/tokens.css passes" \
  '{"hook_event_name":"PreToolUse","tool_name":"Write","cwd":"/proj","tool_input":{"path":"/proj/design/tokens.css","new_str":":root{--color-bg:#0B0B0F;}"}}'

assert_pass \
  "Read tool is ignored" \
  '{"hook_event_name":"PreToolUse","tool_name":"Read","cwd":"/proj","tool_input":{"path":"/proj/style.css"}}'

assert_pass \
  "color literal inside a comment is not a violation" \
  '{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/proj","tool_input":{"path":"/proj/style.css","new_str":"/* the CTA button should NOT be #FF0000 */\n.hero{color:var(--color-text);}"}}'

assert_deny \
  "named color hotpink in tsx is denied" \
  "raw color" \
  '{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/proj","tool_input":{"path":"/proj/Hero.tsx","new_str":"const s = { color: \"hotpink\" };"}}'

echo
if [ "$fail" -gt 0 ]; then
  printf '%d passed, %d FAILED\n' "$pass" "$fail"
  exit 1
fi
printf '%d passed, 0 failed\n' "$pass"
