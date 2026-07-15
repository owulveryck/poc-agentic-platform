# Demo skill package (APM)

An [APM](https://github.com/microsoft/apm) package containing three skills:

| Skill | Tier | What it does |
|---|---|---|
| `ppg-tutorial` | 2 (Bash) | Runs the amplified planning loop demo on its own: gateway, enrich, deterministic rejection, capability ticket, out-of-scope refusal, debt report, and the GitHub Copilot pre-flight variant, narrating every real transcript |
| `add-payment-method` | 1 (Edit) | The governed workflow from the companion article: enrich the plan with the platform ADRs, lock it for a capability ticket, implement within the ticket scope. Authored step by step in [tutorial 6](../docs/tutorials/06-skill-to-session-end-to-end.md) |
| `design-system` | 1 (Edit) | Applies the Deep Umbra design system (canonical `tokens.css` + button rule) and enforces every subsequent UI edit through a shell-script PreToolUse hook shipped inside the skill. Walkthrough in [tutorial 8](../docs/tutorials/08-design-system-end-to-end.md); the enforcement pattern is generalized in [Enforce a content invariant](../docs/how-to/enforce-a-content-invariant.md) |

## Install

From the project where you want the skill (APM ≥ 0.23):

```bash
# For Claude Code (deploys to .claude/skills/)
apm install owulveryck/poc-agentic-platform/demo --target claude

# For GitHub Copilot and other agents reading the cross-agent standard
# location (deploys to .agents/skills/)
apm install owulveryck/poc-agentic-platform/demo --target copilot
```

A local checkout works the same way:
`apm install /path/to/poc-agentic-platform/demo --target claude`.

Then invoke them from a session:

```
/ppg-tutorial Add Stripe as a payment method to the checkout service
/add-payment-method Stripe
/design-system Build a landing page with a big START PAYMENT CTA button
```

`ppg-tutorial` needs Go 1.25+ and network access to `localhost:8765` (it
starts the gateway itself if none is running); `add-payment-method` expects
the full tutorial-2 or tutorial-7 wiring (MCP server + hooks);
`design-system` bootstraps itself on first invocation (copies
`tokens.css` to `design/tokens.css` and registers its PreToolUse hook in
`.github/hooks/design.json`).

## Dogfooding: the skill passes its own gate

This skill is a dual-representation artifact like everything else in this
repository: `SKILL.md` (the semantic workflow) plus `SKILL.rego` (the
companion policy). It validates through the platform's own publish gate:

```bash
curl -s -X POST localhost:8765/validate_skill \
  -H "Content-Type: application/json" --data @payload.json
# → {"status": "SKILL_VALID", "tier": 2}
```

Tier 2 (the body instructs `Bash`): in a real registry this skill would
require human review before publication. See
[docs/how-to/gate-skill-publication-in-ci.md](../docs/how-to/gate-skill-publication-in-ci.md)
for the CI recipe and
[docs/reference/skill-governance.md](../docs/reference/skill-governance.md)
for the tiers.
