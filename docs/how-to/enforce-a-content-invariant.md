# How to enforce a content invariant with a PreToolUse hook

> Solves one problem: preventing specific *values* — not specific files
> — from ever being written by an agent. Brand colors outside a
> palette, i18n keys missing from the catalog, license headers absent
> from new source files, deprecated APIs sneaking back in — anywhere
> the invariant lives in the emitted bytes.
>
> Worked example: [tutorial 8](../tutorials/08-design-system-end-to-end.md)
> ships this pattern as a design-system enforcer.

## When to use this vs. the alternatives

Pick the right layer:

| The invariant is about… | Use |
|---|---|
| Which **files** may be modified in this task | Capability ticket + `ppg-*-guard` (path scope) — [tutorial 7](../tutorials/07-copilot-end-to-end.md) |
| The **structure** of the plan (must have a test step, must include a migration) | Rego plan-linter policy at `lock_in_plan` — [Rego survival kit](rego-survival-kit.md) |
| The **values** written to a file (raw colors, secrets, forbidden strings, missing headers) | **This how-to.** Content-scope PreToolUse hook. |
| The **runtime behavior** of the code after it's written (type errors, syntax, semantic checks) | Smart tool at `/tools/{name}` returning `remediation_guidance` — [Add a smart tool](add-a-smart-tool.md) |

Content-scope hooks are cheap and composable — you can register several
on the same event, and the most restrictive `deny` wins.

## Anatomy of a content-scope hook

A hook is any executable that:

1. Reads a JSON payload on **stdin**.
2. Writes a JSON decision on **stdout**.
3. Exits 0.

That's the entire contract. The language is a distribution choice, not
an architectural one — bash + `jq`, Python, Deno, a compiled Go binary
all work. The design system's `design-guard.sh` is ~60 lines of shell;
`ppg-copilot-guard` is Go because it needed unit tests and reuse of the
JWT verification code. Pick what fits the check.

### The payload you receive on stdin

For a Copilot `Edit` action:

```json
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Edit",
  "session_id": "…",
  "cwd": "/absolute/project/path",
  "tool_input": {
    "path": "/absolute/project/path/style.css",
    "old_str": "…",
    "new_str": "…"
  }
}
```

For `Write` and `editFiles` (VS Code Copilot Chat), the shape is very
similar — `tool_input.file_path` is used instead of `path` in some
cases. Extract both to be safe:

```bash
FILE_PATH=$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.path // .tool_input.file_path // empty')
NEW_STR=$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.new_str // .tool_input.content // empty')
```

### The decision you emit on stdout

Pass through (do not interfere):

```json
{"continue":true}
```

Deny (block the edit; the reason is surfaced to the model and the user):

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "SEMANTIC_CODE: one-line reason. Enough for the model to fix precisely that."
  }
}
```

Rules of thumb for the reason string:

- Prefix with a stable error code (`DESIGN_SYSTEM_VIOLATION`,
  `MISSING_LICENSE_HEADER`, `FORBIDDEN_API`). The model will pattern-match on it.
- Name what was seen (`"#F0F"`, `console.log`, the missing key).
- Name the paved path (`use var(--color-primary)`, `add the header from
  scripts/license-header.txt`, `use the wrapper in internal/log`).
- End with a stance about state (`Nothing was modified.`) so the model
  knows it isn't looking at a partial write.

## A minimal template

Save as `hooks/my-guard.sh`, `chmod +x`, register in
`.github/hooks/*.json`:

```bash
#!/bin/bash
set -u
PAYLOAD="$(cat)"

emit_allow() { printf '%s\n' '{"continue":true}'; exit 0; }
emit_deny() {
  printf '%s' "$1" | jq -Rs '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: .
    }
  }'
  exit 0
}

command -v jq >/dev/null 2>&1 || emit_allow  # broken harness → stay out of the way

TOOL_NAME=$(printf '%s' "$PAYLOAD" | jq -r '.tool_name // empty')
FILE_PATH=$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.path // .tool_input.file_path // empty')
NEW_STR=$(printf '%s'   "$PAYLOAD" | jq -r '.tool_input.new_str // .tool_input.content // empty')

case "$TOOL_NAME" in Edit|Write|editFiles) ;; *) emit_allow ;; esac
[ -z "$FILE_PATH" ] || [ -z "$NEW_STR" ] && emit_allow

# --- your check goes here ---------------------------------------------
# Example: forbid `console.log` in JS/TS source
case "$FILE_PATH" in *.js|*.ts|*.tsx|*.jsx) ;; *) emit_allow ;; esac
if printf '%s' "$NEW_STR" | grep -qE 'console\.(log|debug)\('; then
  emit_deny "FORBIDDEN_API: console.log/debug is banned in shipped code. Use the wrapper in internal/log (log.Debug / log.Info). Nothing was modified."
fi
# ----------------------------------------------------------------------

emit_allow
```

Register it:

```json
// .github/hooks/no-console.json
{
  "hooks": {
    "PreToolUse": [
      { "type": "command",
        "command": "./hooks/my-guard.sh",
        "timeoutSec": 5 }
    ]
  }
}
```

## Test it — with fixtures, no framework

Trust nothing until you've fed hand-crafted payloads to the hook and
seen the exact decision. This is the same pattern
`demo/skills/design-system/hooks/design-guard-test.sh` uses:

```bash
#!/bin/bash
set -eu
GUARD=./hooks/my-guard.sh

assert_deny() {  # name, want_reason_fragment, payload
  out=$(printf '%s' "$3" | "$GUARD")
  d=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.permissionDecision // "?"')
  r=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.permissionDecisionReason // ""')
  [ "$d" = "deny" ] && printf '%s' "$r" | grep -qF "$2" && echo "PASS $1" || echo "FAIL $1"
}

assert_pass() {  # name, payload
  out=$(printf '%s' "$2" | "$GUARD")
  d=$(printf '%s' "$out" | jq -r '.continue // .hookSpecificOutput.permissionDecision // "?"')
  [ "$d" = "true" ] && echo "PASS $1" || echo "FAIL $1"
}

assert_deny "console.log denied" "FORBIDDEN_API" \
  '{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/p",
    "tool_input":{"path":"/p/x.ts","new_str":"console.log(x);"}}'

assert_pass "log.Debug allowed" \
  '{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/p",
    "tool_input":{"path":"/p/x.ts","new_str":"log.Debug(x);"}}'
```

Fixture tests double as documentation: reading them tells another
maintainer exactly what the hook does and doesn't gate.

## Compose with other hooks

Multiple hooks per event are supported. The Copilot / VS Code runtime
runs them in parallel and applies the **most restrictive** decision
(`deny` beats `ask` beats `allow`). Tutorial 8 combines two hooks on
`PreToolUse`:

- `ppg-copilot-guard` — path scope (ticket-driven)
- `design-guard.sh` — content scope (design-system)

Neither knows about the other. If either denies, the edit is refused.

## When to escalate from shell to a compiled binary

Move the check out of shell into `adapters/copilot/<name>/` (Go, or
whatever the platform team already builds) when any of these are true:

- The check needs a **real parser** (CSS AST, TS type-checker, semantic
  diff, JSON Schema validation) — regex won't cut it.
- The check needs to **query external state** (a Rego policy in the
  gateway, a rules server, a schema registry) with retries, timeouts,
  and error handling.
- The check is used by **multiple projects** and duplicating a shell
  script across them creates drift risk.
- You want **robust unit tests** that run in CI on every PR.

The mechanism is the same either way — JSON in, JSON out. Only the
language changes. `adapters/copilot/guard/main.go` is a working
reference for the Go shape.

## Known limits

- **Regex is not a parser.** The design guard treats `#F0F` inside a
  string literal (`const label = "#F0F5FA is the primary hue";`) as a
  color literal. For code contexts where this matters, escalate.
- **`old_str` is not checked.** Only `new_str` (the incoming content)
  matters — the delete side of an Edit is unrestricted. If you need to
  gate deletes too, inspect `old_str` as well.
- **Hooks are Preview.** The Copilot hooks format is marked preview in
  the VS Code docs; the payload shape may change. Pin the docs URL in
  a comment at the top of your hook.
- **Hooks are per-repo unless registered globally.** Content-scope
  guards live in `.github/hooks/*.json` — that's a per-project
  registration. To enforce the same invariant across every project,
  ship the hook inside a skill that projects `apm install` (design-system
  does this), or a plugin that registers itself on install.
