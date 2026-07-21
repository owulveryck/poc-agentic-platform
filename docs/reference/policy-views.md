# Policy views

> Factual and exhaustive. For the "why" of three altitudes, see
> [the dual-representation ADR](../explanation/dual-representation-adr.md).
> For a from-scratch Rego walkthrough, see the
> [Rego survival kit](../how-to/rego-survival-kit.md).

The validation server evaluates the same Rego corpus at three altitudes. Rules
discriminate between them by reading the `input.view` field. A single
`.rego` file can carry rules for any subset of the three views; the ADR's
[front matter](adr-front-matter.md) `enforcement.altitudes` field declares
which ones it opts into.

## The three views

| View | Endpoint | Fires when | The rule reads |
|---|---|---|---|
| `plan` | [`POST /lock_in_plan`](http-api.md#post-lock_in_plan) | The agent submits a plan for locking | `input.steps[]`, `input.intent`, `input.skill_id`, `input.session_id`, `input.repository_context` |
| `artifact` | [`POST /verify_artifact`](http-api.md#post-verify_artifact) | The `ppg-guard` / `ppg-copilot-guard` `PreToolUse` hook intercepts an `Edit`/`Write`; Smart Tools also call this over their payload | `input.artifact` — `{path, content, op}` |
| `changeset` | [`POST /verify_changeset`](http-api.md#post-verify_changeset) | `ppg-verify` runs (pre-commit / pre-push / CI); the apply-time backstop for hookless surfaces | `input.changeset` — `{files: [{path, content, op}], plan_hash?}` |

## Input schemas

The Go source of these shapes is
[`internal/linter/linter.go`](../../internal/linter/linter.go)
(`planInput`, `artifactInput`, `changesetInput`).

### `plan` view

```json
{
  "view": "plan",
  "intent": "add rate limiting to the public API",
  "session_id": "11111111-1111-1111-1111-111111111111",
  "skill_id": "design-system",
  "repository_context": {"name": "checkout", "tech_stack": ["Go"]},
  "steps": [
    {"id": "s1", "action": "read design tokens",
     "tool": "Read", "targets": ["design/tokens.css"]},
    {"id": "s2", "action": "write component",
     "tool": "Write", "targets": ["src/Button.tsx"]}
  ]
}
```

Full plan shape: [plan-contract.md](plan-contract.md).

### `artifact` view

```json
{
  "view": "artifact",
  "artifact": {
    "path": "src/Button.tsx",
    "content": "…full proposed content of the file after the edit…",
    "op": "edit"
  }
}
```

One file at a time. The guard hook posts one of these per intercepted
`Edit`/`Write` call.

### `changeset` view

```json
{
  "view": "changeset",
  "changeset": {
    "plan_hash": "sha256:44c781b1…",
    "files": [
      {"path": "src/Button.tsx", "content": "…"},
      {"path": "design/tokens.css", "content": "…"}
    ]
  }
}
```

The whole diff in one call. `plan_hash` lets the validation server detect plan
substitution against the ticket claim; leave it out for a bare content
check.

## Writing view-aware rules

Guard each rule with the view it targets:

```rego
package ppg.linter

import rego.v1

# Plan altitude: no plan may target the frozen legacy directory.
violation contains v if {
  input.view == "plan"
  some step in input.steps
  startswith(step.targets[_], "internal/auth/")
  v := {
    "policy_id": "frozen_paths_enumeration",
    "message":   "internal/auth/ is frozen; do not modify it.",
    "nature":    "compensatory",
  }
}

# Artifact + changeset altitudes: forbid raw hex colors in UI files, wherever
# they come from. governed_files unifies the two content views so one rule
# set covers both.
governed_files contains f if {
  input.view == "artifact"
  f := input.artifact
}
governed_files contains f if {
  input.view == "changeset"
  some file in input.changeset.files
  f := file
}
violation contains v if {
  some f in governed_files
  endswith(f.path, ".tsx")
  regex.match(`#[0-9a-fA-F]{3,8}\b`, f.content)
  v := {
    "policy_id": "design_tokens_referenced",
    "message":   sprintf("%s uses a raw hex color.", [f.path]),
    "nature":    "amplifier",
  }
}
```

The canonical multi-altitude ADR is
[`examples/adr/ADR-090.rego`](../../examples/adr/ADR-090.rego); read it in
full when writing your first content-altitude rule.

## SKILL.rego uses the same views

Since v1.0.0, a published skill's companion `SKILL.rego` is evaluated at
all three altitudes just like an ADR, with **union semantics** at the
content altitudes:

- **Plan view** — selected by declaration. When a plan locks with
  `skill_id: "<name>"`, that skill's plan-view rules are evaluated
  (fail-closed: an unknown id rejects the plan). A skill's plan rules are
  *workflow requirements* ("the plan must read the tokens file"), which
  only make sense for plans executed under that skill.
- **Artifact and changeset views** — union. Every registered skill
  applicable to the session (operator tier + this session's uploads) is
  evaluated against every edit and every diff, **whether or not the plan
  declared that skill**. A skill's content rules are *invariants*, and an
  installed skill's validation applies automatically — a plan that omits
  `skill_id` does not bypass them. The declared skill (if any) is
  evaluated once, by the fail-closed path.

The reference companion is
[`demo/skills/design-system/SKILL.rego`](../../demo/skills/design-system/SKILL.rego)
— structurally identical to ADR-090.rego. The two coexist: an ADR is
org-wide policy, a `SKILL.rego` travels with the skill; both traverse the
same three views.

## Where a SKILL.rego comes from

There are two tiers, consulted in this order at every view:

| Tier | Source | Loaded by | Persistence | Scope |
|---|---|---|---|---|
| **Operator** | `ppg -skills <dir>` at startup | `linter.LoadSkillCompanions` | Re-read at every validation server startup from disk | Global — every session sees it |
| **Session-scoped** | `POST /register_skill` from the MCP server | `linter.RegisterSessionSkill` | In-memory only — dropped on validation server restart | Only the `session_id` in the request |

The MCP server ([`ppg-mcp-server`](validation-server-cli.md)) auto-uploads every
skill it finds before forwarding `lock_in_plan`, scanning three roots in
ascending precedence (last wins on a duplicate name, because the session
tier is last-write-wins):

1. `~/.claude/skills/` — user-wide installs, the governed-workstation
   location;
2. `<project>/.agents/skills/` — the cross-agent directory
   ([agent-skills.io](https://agent-skills.io/)), shared with Copilot;
3. `<project>/.claude/skills/` — project-local Claude Code skills.

That's how a skill installed by `apm install ... --target claude` — into
the project *or* user-wide — reaches a validation server that does not share the
client's filesystem (e.g. a shared team validation server or a container).

**Post-restart recovery.** Because session-scoped skills are memory-only,
a validation server restart mid-session leaves the MCP's upload cache stale. The
MCP handles this automatically: any `/lock_in_plan` response containing
an `unknown_skill` violation triggers a cache invalidation and a re-upload
of the named skill, then a single retry of the lock. Bounded at one
retry — see
[`/register_skill` lifetime](http-api.md#lifetime--post-restart-recovery).

**Precedence rule**: on a name collision the operator tier wins. This
prevents a project-local upload from silently downgrading an org-wide
policy the operator has already reserved under the same name.

**Multi-user posture.** The validation server trusts `session_id` from the client
and is safe as a shared, single-tenant service on a trusted network. See
the [multi-user posture note](http-api.md#authentication--multi-user-posture)
for what an enterprise multi-tenant deployment would add (mTLS, signed
manifests).

## Failure modes

| Situation | Outcome |
|---|---|
| Rule iterates `input.steps` at artifact view without a view guard | Rule silently no-ops (`input.steps` is undefined). Safe but confusing — always add `input.view == "…"` |
| Ticket declares `skill_id` for a skill the validation server does not know about | `unknown_skill` violation at every view — fail-closed. Fix: start the validation server with `-skills` pointing at the directory containing the skill package |
| Skill was published without a `SKILL.rego` (tier 0) | Skill loads with a nil evaluator; the companion is a no-op at every view. The ADR corpus still applies |
| Guard hook cannot reach the validation server | The guard fails closed with `PPG_GUARD_ERROR: cannot verify content …`. Every write is refused until the validation server is reachable |
| `.rego` fails to compile at startup | `ppg` refuses to start. Fix the syntax and restart |
