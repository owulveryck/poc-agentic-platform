#!/bin/bash
# design-guard.sh — PreToolUse hook enforcing the Deep Umbra design system.
#
# Contract: reads a JSON payload on stdin, emits a JSON decision on stdout.
#   Pass: {"continue":true}
#   Deny: {"hookSpecificOutput":{"hookEventName":"PreToolUse",
#                                "permissionDecision":"deny",
#                                "permissionDecisionReason":"..."}}
#
# Two checks (both produce DESIGN_SYSTEM_VIOLATION on deny):
#   A. Colors  — no raw hex/rgb()/hsl()/named-color outside design/tokens.css
#                or a var(--...) reference or a CSS keyword.
#   B. Buttons — the canonical button rule lives in design/tokens.css;
#                any button-selector rule in another file is denied.
#
# The hook is intentionally shell + jq + grep to keep the mechanism
# inspectable. See docs/how-to/enforce-a-content-invariant.md for the
# escalation path to a compiled binary.

set -u

PAYLOAD="$(cat)"

emit_allow() {
  printf '%s\n' '{"continue":true}'
  exit 0
}

emit_deny() {
  # $1: the semantic reason
  local reason="$1"
  # Escape the reason for JSON. jq handles this correctly.
  printf '%s' "$reason" | jq -Rs '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: .
    }
  }'
  exit 0
}

# Extract fields. jq is required; if unavailable, the hook stays out of
# the way (broken harness must not lock the session).
if ! command -v jq >/dev/null 2>&1; then
  emit_allow
fi

TOOL_NAME=$(printf '%s' "$PAYLOAD" | jq -r '.tool_name // empty')
FILE_PATH=$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.path // .tool_input.file_path // empty')
NEW_STR=$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.new_str // .tool_input.content // empty')

# Only gate edit-shaped tools. Copilot names include Edit/Write; VS Code
# Copilot Chat uses editFiles.
case "$TOOL_NAME" in
  Edit|Write|editFiles) ;;
  *) emit_allow ;;
esac

# Nothing to inspect: pass through.
if [ -z "$FILE_PATH" ] || [ -z "$NEW_STR" ]; then
  emit_allow
fi

# Only gate UI file types.
case "$FILE_PATH" in
  *.html|*.css|*.tsx|*.jsx|*.svelte|*.vue) ;;
  *) emit_allow ;;
esac

# Bootstrap: the tokens file itself is exempt. Match by basename to be
# forgiving of the reader's directory layout.
BASENAME_PATH="${FILE_PATH##*/}"
DIR_PATH="${FILE_PATH%/*}"
IS_TOKENS_FILE=0
if [ "$BASENAME_PATH" = "tokens.css" ] && [ "${DIR_PATH##*/}" = "design" ]; then
  IS_TOKENS_FILE=1
fi

# Strip comments so raw-color hits inside /* ... */ or // ... don't
# false-positive. Portable-ish: awk removes block comments, sed strips
# line comments.
CLEAN=$(printf '%s' "$NEW_STR" | awk '
  BEGIN { in_block = 0 }
  {
    line = $0
    out = ""
    i = 1
    while (i <= length(line)) {
      if (in_block) {
        end = index(substr(line, i), "*/")
        if (end == 0) { i = length(line) + 1 } else { in_block = 0; i = i + end + 1 }
      } else {
        start = index(substr(line, i), "/*")
        if (start == 0) { out = out substr(line, i); i = length(line) + 1 } else {
          out = out substr(line, i, start - 1)
          in_block = 1
          i = i + start + 1
        }
      }
    }
    print out
  }
' | sed 's|//.*$||')

# ---------------------------------------------------------------------
# Check B — button rule outside design/tokens.css
# ---------------------------------------------------------------------
if [ "$IS_TOKENS_FILE" -eq 0 ]; then
  if printf '%s' "$CLEAN" | grep -qE '(^|[^-a-zA-Z0-9])(button|\.btn|\.button|\[role="button"\])[[:space:]]*[{,]'; then
    emit_deny "DESIGN_SYSTEM_VIOLATION: button styling belongs in design/tokens.css only. The design system's <button> rule is canonical — use <button> or class=\"btn\" in markup, do not re-style buttons in ${FILE_PATH}. If a new button variant is genuinely needed, extend design/tokens.css."
  fi
fi

# ---------------------------------------------------------------------
# Check A — raw color literal outside var() and outside design/tokens.css
# ---------------------------------------------------------------------
if [ "$IS_TOKENS_FILE" -eq 1 ]; then
  emit_allow
fi

# Scan line-by-line so we can suppress hits that live inside var(--...).
VIOLATION=""
while IFS= read -r line; do
  # Skip lines whose only color-shaped tokens live inside var(--...).
  # Cheap heuristic: if the line contains "var(--" AND at least one hit,
  # trust that the hit is inside var(). Refine if this proves too loose.
  hex_hit=$(printf '%s' "$line" | grep -oE '#[0-9a-fA-F]{3,8}\b' | head -1)
  func_hit=$(printf '%s' "$line" | grep -oiE '(rgb|rgba|hsl|hsla|hwb|lab|lch)\(' | head -1)
  named_hit=$(printf '%s' "$line" | grep -owiE '(red|white|black|hotpink|pink|cyan|magenta|yellow|lime|orange|purple|brown|gray|grey|blue|green|silver|gold|violet|indigo)' | head -1)

  hit="${hex_hit}${func_hit}${named_hit}"
  [ -z "$hit" ] && continue

  # If the line uses var() at all, assume the raw literal is either a
  # var() fallback (arguably fine but we're strict) OR the color hit is
  # inside another declaration on the same line. To stay strict but
  # avoid false positives, require the hit to appear OUTSIDE any var()
  # region: strip var(--...) segments and re-scan.
  stripped=$(printf '%s' "$line" | sed -E 's/var\(--[^)]*\)//g')
  hex_left=$(printf '%s' "$stripped" | grep -oE '#[0-9a-fA-F]{3,8}\b' | head -1)
  func_left=$(printf '%s' "$stripped" | grep -oiE '(rgb|rgba|hsl|hsla|hwb|lab|lch)\(' | head -1)
  named_left=$(printf '%s' "$stripped" | grep -owiE '(red|white|black|hotpink|pink|cyan|magenta|yellow|lime|orange|purple|brown|gray|grey|blue|green|silver|gold|violet|indigo)' | head -1)
  hit_left="${hex_left}${func_left}${named_left}"

  if [ -n "$hit_left" ]; then
    VIOLATION="$hit_left"
    break
  fi
done <<EOF
$CLEAN
EOF

if [ -n "$VIOLATION" ]; then
  emit_deny "DESIGN_SYSTEM_VIOLATION: raw color \"${VIOLATION}\" is not part of the Deep Umbra palette. Use one of: var(--color-primary), var(--color-accent), var(--color-warning), var(--color-success), var(--color-danger), var(--color-text), var(--color-bg), var(--color-surface), var(--color-muted) — all defined in design/tokens.css. Nothing was modified."
fi

emit_allow
