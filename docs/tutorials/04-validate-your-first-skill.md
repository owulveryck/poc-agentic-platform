# Tutorial — validate your first skill

> **Goal**: run a skill through the governance gate (`POST /validate_skill`),
> see it rejected with actionable violations, fix it, and watch the security
> tier change with the tools the skill mentions.
>
> Time: ~5 minutes. Prerequisites: the validation server running (tutorial 1, step 1).
> Note the startup line `Skill governance linter ready`: the policies come
> from the `skill-governance/` directory (`-skill-governance` flag).

## Step 1 — Submit a deliberately bad skill

```bash
curl -s -X POST localhost:8765/validate_skill \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Patch Payment",
    "description": "patches stuff",
    "body": "Use Edit to patch the router."
  }' | python3 -m json.tool
```

**What you should observe**: a `422 SKILL_REJECTED` with five violations:

```json
{
    "status": "SKILL_REJECTED",
    "violations": [
        {
            "field": "description",
            "message": "description must be at least 50 characters to be discoverable",
            "nature": "amplifier"
        },
        {
            "field": "description",
            "message": "description must start with a third-person verb (e.g. 'Adds', 'Runs', 'Applies')",
            "nature": "amplifier"
        },
        {
            "field": "name",
            "message": "name must be lowercase-kebab-case and at most 32 characters",
            "nature": "amplifier"
        },
        {
            "field": "rego_policy",
            "message": "Skills that instruct file modifications (tier ≥ 1) must include a companion SKILL.rego declaring their plan governance requirements.",
            "nature": "amplifier"
        },
        {
            "field": "version",
            "message": "version is required for registry publication (semver, e.g. '1.0.0')",
            "nature": "amplifier"
        }
    ],
    "guidance": "Fix the violations above before publishing the skill to the registry."
}
```

👉 *Same register as the plan linter: not "no", but "here is what is
missing". Each violation carries its `nature` on the durability axis — all
five rules here are amplifiers, durable SDLC invariants.*

## Step 2 — Fix and resubmit

```bash
curl -s -X POST localhost:8765/validate_skill \
  -H "Content-Type: application/json" \
  -d '{
    "name": "patch-payment",
    "description": "Applies targeted changes to the payment service, following platform ADRs for proxy and migration ordering.",
    "version": "1.0.0",
    "argument_hint": "<change description>",
    "body": "Lock the plan through lock_in_plan, then use Edit to apply the change described in $ARGUMENTS.",
    "rego_policy": "package ppg.skills.patch_payment\n\nimport rego.v1\n"
  }' | python3 -m json.tool
```

**What you should observe**:

```json
{
    "status": "SKILL_VALID",
    "tier": 1
}
```

👉 *The skill mentions `Edit`, so it is tier 1 (file modifications) and the
companion `rego_policy` is mandatory — the plan linter can enforce the
skill's requirements at `lock_in_plan` time.*

## Step 3 — Explore the tiers

Resubmit the valid skill three times, changing only the `body`:

| Body | Result |
|---|---|
| `"Use Read and Grep to inspect the payment routing code and summarise it."` | `{"status":"SKILL_VALID","tier":0}` |
| `"Use Edit to apply the fix to the payment router."` | `{"status":"SKILL_VALID","tier":1}` |
| `"Use Bash to run the integration test suite."` | `{"status":"SKILL_VALID","tier":2}` |

👉 *Tier 0 = read-only (auto-approvable), tier 1 = file modifications
(CI + Rego gate), tier 2 = shell access (human review). The exact derivation
rules — and their deliberate PoC naivety — are in the
[reference](../reference/skill-governance.md).*

## Where this runs in real life

This endpoint is **Gate 1** of a governed skills registry: the CI of every
skill repository calls it before publication, so no capability reaches the
organization without passing governance. The recipe is in
[gate-skill-publication-in-ci.md](../how-to/gate-skill-publication-in-ci.md);
the *why* is in
[capability-plane-governance.md](../explanation/capability-plane-governance.md).

**✅ Done.** To add your own governance rule, see
[add-a-skill-governance-rule.md](../how-to/add-a-skill-governance-rule.md).
To see a validated skill drive a full Claude Code session through every
control point, continue with
[tutorial 6](06-skill-to-session-end-to-end.md).
