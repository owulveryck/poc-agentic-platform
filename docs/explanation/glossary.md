# Glossary

Canonical vocabulary per [ADR-130](../decisions/ADR-130-gateway-naming.md).
The published blog articles predate this glossary and may use "gateway"
for what is now called the *validation server*.

## Core terms

| Term | Definition | Where it lives in the code |
|---|---|---|
| **Governance harness** | The whole machine-level system — control points + MCP servers + validation server — that governs every project on the workstation. | The sum of `adapters/`, `cmd/ppg`, and the setup scripts. (Not to be confused with `IsHarnessMetadata` in `internal/smarttools`, where "harness" means the *agent runtime*.) |
| **Control point** (gateway) | One individual deterministic enforcement point. Enforcement is *distributed* across control points; they all delegate policy evaluation to the validation server — distributed enforcement, centralized decision. | `ppg-guard` and `ppg-copilot-guard` (in-loop, PreToolUse), the in-tool ticket check (`internal/smarttools`), `ppg-verify` (apply time). |
| **Validation server** | The central policy evaluator: loads the corpus, lints plans, mints capability tickets, verifies artifacts and changesets. | `cmd/ppg` (binary `ppg`). |
| **Skill (with validation)** | A capability bundled with its own validation: a human-facing `SKILL.md` (the invariant, in prose) plus a machine-enforced companion `SKILL.rego`. The rule and its enforcement travel together in one installable package. | `internal/skill`; `demo/skills/*/SKILL.{md,rego}`; `skill_id` in the plan and ticket. |
| **Invariant** (rule) | The human-facing intent — *what* must hold ("buttons must be blue"), stated in domain terms. Called "rule" in some external material. | `internal/adr` (`Invariant`), the `## Invariant` section of every ADR, skill bodies. |
| **Policy** | The deterministic, machine-enforced encoding of one or more invariants, written in OPA/Rego — *how* conformance is checked. Rego is the only supported policy format. | `internal/policy`; `*.rego` next to ADRs; `SKILL.rego`; `skill-governance/`. |
| **Plan** | The plan step of the agentic loop: the agent's structured statement of intended actions, validated *before* anything executes (`lock_in_plan`). | `internal/plan`, `schemas/plan.schema.json`, `POST /lock_in_plan`. |
| **Capability ticket** ("locked plan") | The materialization of a locked plan: an ephemeral signed token carrying the plan fingerprint, a least-privilege file/tool scope, the session binding, and the declared skill. No ticket, no edit. Note: it freezes *scope*, not step order — `depends_on` is cycle-checked at lock time only. | `internal/ticket` (JWT claims `plan_hash`, `scope`, `session_id`, `skill_id`). |
| **LLM-judge validation** | The rejected alternative: a second LLM reviewing the first one's output (sometimes called "adversarial"). Non-deterministic, per-validation token cost, non-capitalizable, degrades as rules accumulate. | Nowhere, by design — see [design decisions](design-decisions-and-limits.md). In tutorials, "adversarial" instead describes hostile *prompts* used in demos. |

## Supporting vocabulary

| Term | Definition |
|---|---|
| **View / altitude** | The three moments the same policy corpus is evaluated at, discriminated by `input.view`: `plan` (lock time), `artifact` (one edit's content, in-loop), `changeset` (the whole diff, apply time). "Altitude" and "view" are synonyms; `input.view` is the mechanical name. |
| **Amplifier / compensatory** | The durability axis of every governance artifact. *Amplifier*: stays valuable as models improve (an architectural invariant). *Compensatory*: works around a current model limitation and must carry a measurable `sunset_condition`. |
| **Transition debt** | The ratio of compensatory artifacts to the total; must trend toward 0. Reported by `GET /debt_report`. |
| **Enrichment** | The soft move of the plan phase: `POST /enrich` returns the architectural invariants relevant to an intent, so the model plans with the rules in context instead of discovering them at rejection time. |
| **Smart Platform Tool** | A platform-provided execution tool that verifies the capability ticket in-tool, evaluates the artifact-view policy over its input, executes in a sandbox, and returns semantic feedback (`remediation_guidance`). |
| **Service catalog** | The discovery plane: `POST /discover_service` returns the sanctioned service for a capability, ranked by a Rego policy; ADR-110 makes the recommendation binding. |
| **Gates 1/2/3** | The skill lifecycle checkpoints: publish-time governance lint (Gate 1, `/validate_skill`), install-time revalidation (Gate 2, not implemented — registry-side), plan-time companion enforcement (Gate 3, `skill_id`). Not to be confused with control points. |
| **Security tier 0/1/2** | Skill privilege classification derived from the tools its body mentions: read-only / file-modifying / shell. Tier ≥ 1 requires a companion `SKILL.rego` at publish time. |
| **Operator vs session skills** | Two sourcing paths for skill companions: operator skills are loaded from `-skills` at server startup (machine-wide, win on name collision); session skills are uploaded by an adapter via `POST /register_skill` and live for the session. |
| **Escalation** | The intended behavior on contradiction: a hard block that no configuration of the agent can bypass, surfacing enough information for a human to fix the underlying policies — feeding the monotonic improvement loop. See the known-limits entry in [design decisions](design-decisions-and-limits.md) for current coverage. |
