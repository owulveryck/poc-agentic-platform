# How to enforce a content invariant

> Solves one problem: preventing specific *values* — not specific files
> — from ever being written by an agent. Brand colors outside a
> palette, i18n keys missing from the catalog, license headers absent
> from new source files, deprecated APIs sneaking back in — anywhere
> the invariant lives in the emitted bytes.
>
> Worked example: [tutorial 8](../tutorials/08-design-system-end-to-end.md)
> ships this pattern as the design-system enforcer, backed by
> `adr/ADR-090.rego`.

## The canonical way: an ADR with an artifact-view rule

A content invariant is just a policy that reads the edited bytes instead
of the plan. Author it the same way as any other invariant — a
dual-representation ADR (Markdown body + paired `.rego`) — and add a
`violation` rule guarded by `input.view == "artifact"` that reads
`input.artifact` (`{path, content, op}`). The platform enforces it for
free, at two altitudes:

- **In-loop** — the standard `ppg-guard` / `ppg-copilot-guard` sends
  every edit's content to the gateway's `/verify_artifact`, which runs
  your rule. On a violation the edit is blocked before the bytes hit
  disk, with your message fed back to the model.
- **Apply-time** — `ppg-verify` sends the whole working-tree diff to
  `/verify_changeset`, which runs the same corpus (changeset view). This
  covers the surfaces with no in-loop hook (`gh copilot` CLI, Cursor, a
  human, CI). See [Gate changes at apply time](gate-changes-at-apply-time.md).

No per-project hook to register, no shell script to ship: the workstation
guard (tutorial 0) already wires both endpoints. You write Rego; the
platform runs it at every altitude.

## When to use this vs. the alternatives

Pick the right layer:

| The invariant is about… | Use |
|---|---|
| Which **files** may be modified in this task | Capability ticket + `ppg-*-guard` (path scope) — [tutorial 7](../tutorials/07-copilot-end-to-end.md) |
| The **structure** of the plan (must have a test step, must include a migration) | Rego plan-linter policy (`input.view == "plan"`) at `lock_in_plan` — [Rego survival kit](rego-survival-kit.md) |
| The **values** written to a file (raw colors, secrets, forbidden strings, missing headers) | **This how-to.** An artifact-view Rego rule. |
| The **runtime behavior** of the code after it's written (type errors, syntax, semantic checks) | Smart tool at `/tools/{name}` returning `remediation_guidance` — [Add a smart tool](add-a-smart-tool.md) |

## Worked example — ADR-090's content rules

`adr/ADR-090.rego` is the reference. Its plan-altitude rule requires a
`design/tokens.css` read step; its **content** rules unify the artifact
and changeset views so one rule set covers both altitudes:

```rego
package ppg.linter

import rego.v1

# governed_files unifies the two content views: one edit (artifact) and a
# whole diff (changeset). A single rule set then covers both altitudes.
governed_files contains f if {
    input.view == "artifact"
    f := input.artifact
}

governed_files contains f if {
    input.view == "changeset"
    some file in input.changeset.files
    f := file
}

# The invariant: no raw color value in a UI file (except design/tokens.css).
violation contains v if {
    some f in governed_files
    governed_ui_file(f)
    raw_color_present(lower(f.content))
    v := {
        "policy_id": "design_tokens_referenced",
        "message":   sprintf("Design-system invariant (%s): raw color value found. Reach colors through design tokens (var(--color-*)) or a CSS keyword; raw hex, rgb()/hsl(), and named colors are forbidden outside design/tokens.css.", [f.path]),
        "nature":    "amplifier",
    }
}

governed_ui_file(f) if {
    is_ui_path(f.path)
    f.path != "design/tokens.css"   # the tokens file is where raw values live
}
```

The key moves, all reusable for your own invariant:

1. **Read the content, not the plan.** `input.artifact.content` (one
   edit) or `input.changeset.files[_].content` (the diff). The
   `governed_files` helper above lets one `violation` rule serve both.
2. **Scope by path.** `governed_ui_file` restricts the check to the file
   types the invariant applies to, and exempts the one file where the
   raw values legitimately live.
3. **Write the message for the model.** Name what was seen, name the
   paved path, and make it a single actionable line — it is what the
   guard feeds back on a block.
4. **Match robustly.** ADR-090 does not strip `var()` before checking, so
   a raw color hidden in a `var(--x, #F0F)` fallback is still caught; its
   button rule matches `button:hover`, `button > span`, `.btn`, and
   `[role="button"]`. Regex is not a parser (see limits) — but a careful
   rule closes the obvious bypasses.

Declare the altitudes your `.rego` implements in the ADR front matter so
the catalog documents them:

```yaml
enforcement:
  mode: programmatic
  policy_id: design_tokens_referenced
  rego: ADR-090.rego
  altitudes: [plan, artifact]
```

`altitudes` defaults to `[plan]` when omitted; list `artifact` and/or
`changeset` when your `.rego` has rules for those views. See
[ADR front matter](../reference/adr-front-matter.md) and the
[policy catalog](../reference/policy-catalog.md).

## Ship it

1. Drop `adr/ADR-XXX-your-invariant.md` (front matter + invariant prose)
   and `adr/ADR-XXX.rego` (package `ppg.linter`, your `input.view ==
   "artifact"` rule) next to the existing ADRs.
2. Restart the gateway — it compiles every `adr/*.rego` into the corpus
   at startup. Confirm with the `Plan linter ready: N policies` line.
3. That's it. `ppg-guard` / `ppg-copilot-guard` now block violating edits
   in-loop via `/verify_artifact`; `ppg-verify` catches them at apply
   time via `/verify_changeset`.

Full procedure for adding an ADR (tests included) is in
[Add an ADR invariant](add-an-adr-invariant.md); the Rego primitives are
in the [Rego survival kit](rego-survival-kit.md).

## Verifying your rule

Trust nothing until you've fed the policy hand-crafted content and seen
the decision. Two ways:

- **Unit-test the Rego** with fixtures for each view — a clean artifact,
  a violating artifact, and a changeset with one bad file. This is how
  the platform's own policies are tested (`internal/linter`).
- **Exercise the endpoints** against a running gateway:

  ```bash
  # An in-loop artifact check (ticket from a locked plan)
  curl -s -X POST localhost:8765/verify_artifact \
    -H 'Content-Type: application/json' \
    -d '{"ticket":"<jwt>","path":"style.css","content":"a{color:#f0f}"}'
  # → {"status":"ARTIFACT_REJECTED","violations":[...],"guidance":"..."}
  ```

## The escape hatch: a standalone shell hook for non-Rego cases

The Rego route is the paved road: one rule, enforced at every altitude,
tested and versioned with the ADR corpus. Reach for a bespoke
`PreToolUse` hook only when the check genuinely cannot be expressed
against the corpus — e.g. it needs a real parser (CSS AST, TS
type-checker), or must query external state the gateway does not see.

The contract of such a hook is the platform's own hook contract: read a
JSON payload on **stdin**, decide, and either exit 2 with a stderr
message (Claude Code) or print a `{"permissionDecision":"deny",…}` JSON
on **stdout** (Copilot). `adapters/copilot/guard/main.go` is the working
Go reference for both shapes; a shell script works too. Extract the path
from `tool_input.file_path` / `path` / `notebook_path` and the content
from `tool_input.new_string` / `new_str` / `content`, gate on the
write-tool set, and — like the platform guard — **fail closed** if the
check cannot run. Register it in `.github/hooks/*.json` (Copilot) or
`.claude/settings.json` (Claude Code); multiple hooks per event compose,
most-restrictive `deny` wins.

This is the exception, not the pattern. If the invariant lives in the
bytes and a regex or string match can express it, put it in an ADR's
`.rego` and let the platform enforce it everywhere.

## Known limits

- **Regex is not a parser.** An artifact rule that matches `#F0F` will
  also flag it inside a string literal (`const label = "#F0F5FA is the
  primary hue";`). Where the surrounding syntax matters, escalate to a
  real parser via a smart tool or a bespoke hook.
- **Only the incoming content is seen.** The artifact view carries the
  proposed `content`; the delete side of an edit is not inspected.
- **The in-loop check needs the content field.** The guard only calls
  `/verify_artifact` when the tool payload actually carries the new
  content; a tool that writes without exposing its bytes is caught at
  apply time by `ppg-verify` instead.
