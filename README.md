# PPG — a deterministic governance harness for agentic development loops

PPG wraps an agentic coding loop (Claude Code, GitHub Copilot) in a
**deterministic governance harness**: hooks, MCP servers, and a validation
server, installed at the **machine** level, so that the artifacts an agent
produces **provably respect your rules** *within the governed channel* —
without asking a second LLM to police the first one.

(The name is a historical acronym — *Platform Planning Gateway* — kept as
a brand; per [ADR-130](docs/decisions/ADR-130-gateway-naming.md) the central
component is now called the *validation server*.)

Companion repository of
[The Amplified Agentic Loop](https://blog.owulveryck.info/2026/07/07/amplified-agentic-loop.html)
and *The Governed Skills Registry* (draft). Vocabulary follows
[ADR-130](docs/decisions/ADR-130-gateway-naming.md); see the
[glossary](docs/explanation/glossary.md).

## The problem

Agents run on LLMs, and LLMs are non-deterministic. Nothing guarantees that
the rules you put in the context — instructions, skills, design systems —
will be followed. Worse, when instructions conflict, the outcome is a coin
flip. If your design system says buttons are blue and the developer types
"No, I want them pink", you get pink *or* blue depending on the model's
mood. Human-escalation safeguards don't fix this: they themselves depend on
the model deciding to escalate.

## Why not an LLM judge?

The common mitigation is a second LLM reviewing the first one's output
("LLM-as-a-judge", sometimes called adversarial validation). It helps, but:

- **Cost** — every validation burns tokens.
- **Insufficient trust** — the judge is non-deterministic too; it can let
  the same defect through twice.
- **Not capitalizable** — when a judge misses something, nothing guarantees
  it won't miss it again. When a *deterministic* control point misses
  something, you extend the policy: that defect **never passes again**.
- **Context fragility** — the more rules you load into a judge, the more it
  misses. Reliability degrades exactly as requirements accumulate.

| Criterion | LLM-judge validation | Deterministic harness (this project) |
|---|---|---|
| Determinism | No | Yes |
| Token cost | Per validation | Near zero |
| Trust | Partial, variable | High, reproducible |
| Fixing a defect | Not guaranteed to hold | Permanent (monotonic) |
| Sensitivity to context load | Degrades | Insensitive |
| Bypass possible | Yes (model-dependent) | No within the governed channel (blocking refusal) |
| Scope | Per prompt / per project | Machine level (organization) |

## How it works

A **skill** here is not just a capability with a prose rule ("buttons must
be blue"). It is a capability **bundled with its own validation**: a
human-facing `SKILL.md` plus a machine-enforced `SKILL.rego` (OPA/Rego —
the only supported policy format). The same policy corpus is evaluated at
two moments, deterministically:

1. **At plan time** — before anything executes, the agent's structured plan
   is linted (`lock_in_plan`). A rejected plan gets semantic violations to
   self-correct against; an accepted plan gets a **capability ticket**
   (an ephemeral signed token: plan fingerprint + least-privilege scope).
2. **At artifact time** — every `Edit`/`Write` is intercepted by a control
   point (`ppg-guard`) that checks the ticket and evaluates the produced
   content against the same policies. A third pass (`ppg-verify`) re-checks
   the whole diff at apply/CI time.

No ticket, no edit. Server unreachable, no edit. The blocks are exit codes
and HTTP rejections — not prompts a model can talk its way around.
Enforcement is **distributed** (control points inside the loop, in the
tools, and at apply time); the **decision is centralized** in one
validation server, so every control point renders identical verdicts from
a single policy corpus.

**And when rules contradict each other, the answer is: no action.** A
prompt cannot override a policy — the developer typing "I want them pink"
gets the same refusal on every retry. When an agent livelocks against the
same rejections, the server escalates deterministically
(`409 POLICY_CONFLICT`), naming the clashing policies and appending the
case to an escalation log; `ppg escalations` is how a human inspects the
log, fixes the corpus, and closes the conflict — and once fixed,
that conflict never recurs. That is the monotonic improvement loop: every
escape becomes a permanent policy, not a lost prompt. (Coverage today is
the livelock case; general contradiction detection is undecidable and not
claimed — see the status table.)

## Governed-machine mode (the reference mode)

Install once per workstation; every project on the machine is governed:

```bash
make install                    # binaries into ~/.local/bin
make setup-claude-code          # user-wide hooks + MCP registration
sudo make setup-claude-code-managed   # optional: root-owned, non-overridable hooks
ppg -adr examples/adr                 # the validation server (demo corpus; binds 127.0.0.1:8765)
```

From then on, a skill that embeds a `SKILL.rego` carries its enforcement
with it: the harness uploads and applies it with no per-project
configuration. The rule and its validation travel together — that is what
makes governance capitalizable and distributable.

Architectural invariants (**ADRs**) are an optional second corpus for
organization-wide rules; the harness works without them
(`ppg -skills demo/skills` starts with zero ADRs).

## Try it

```bash
make quickstart        # 1-minute guided demo
```

Then: [tutorial 15](docs/tutorials/15-skill-only-enforcement.md) is the
flagship — one skill, its own Rego, zero ADRs, and an adversarial "make the
buttons pink" prompt refused deterministically. Tutorial
[14](docs/tutorials/14-with-and-without-claude-code.md) is the same demo
with the org-wide corpus; [12](docs/tutorials/12-bypassing-the-gateway.md)
red-teams the whole loop, each bypass paired with its refusal or its
honestly documented limit.

> Want the 30-second overview first? Watch the 90-second animated tour:
> [docs/diagrams/ppg-tutorials-tour.svg](docs/diagrams/ppg-tutorials-tour.svg).

## Status — what is proven vs. in progress

| Promise | Status |
|---|---|
| Deterministic plan + artifact + changeset validation (OPA/Rego, no LLM, restricted to deterministic built-ins) | ✅ implemented, fail-closed, red-teamed |
| Capability ticket, session-bound, plan-hash pinned | ✅ implemented |
| Skill bundled with its own validation, auto-applied | ✅ project and user-wide `.claude/skills` + `.agents/skills` on the MCP path; content policies apply to every edit regardless of the declared `skill_id` |
| Governed-machine install (user + managed scope) | ✅ scripts with dry-run/rollback; managed setup verifies the guard binary is not user-writable |
| ADR-independent operation | ✅ the validation server starts with `-skills` and no ADR corpus |
| Validation-server API authentication | 🟡 none by design; binds `127.0.0.1:8765` by default — a networked or multi-user deployment must front it with an auth proxy (see PoC boundaries) |
| Managed-mode tamper resistance | 🟡 managed scope closes the settings-edit vector; the guard binary (root-owned install) and `PPG_*` env pinning must be hardened operationally against a hostile user (see [set-up-a-governed-workstation](docs/how-to/set-up-a-governed-workstation.md)) |
| Skill validation regardless of declared `skill_id` | 🟡 content-view (artifact/changeset) rules always apply; a skill's plan-view *ordering/workflow* rule is enforced only for a plan that declares its `skill_id` |
| Conflict between validations → blocking escalation | 🟡 deterministic *livelock* escalation: `POLICY_CONFLICT` after 3 rejections with an identical violation set (consecutive or not), blocking every session, surviving restarts, closed only by `ppg escalations resolve`; general unsatisfiability detection is undecidable and not claimed |
| Terminal/Bash writes | 🟡 out of hook reach by design; `ppg-verify` covers them at apply time (`scripts/setup-git-backstop.sh` wires it as a pre-commit hook) |

`AUDIT.md` tracks conformance claim by claim. Known PoC boundaries:
keyword-based retrieval, simulated sandboxes, symmetric per-machine ticket
key, unauthenticated API (bind to localhost).

## Documentation ([index](docs/README.md), Diátaxis)

| You want to… | Read |
|---|---|
| run it and see refusals happen | [docs/tutorials/](docs/tutorials/) |
| install a governed workstation | [docs/how-to/set-up-a-governed-workstation.md](docs/how-to/set-up-a-governed-workstation.md) |
| write a skill with its own validation | [docs/how-to/bundle-validation-with-a-skill.md](docs/how-to/bundle-validation-with-a-skill.md) + [tutorial 15](docs/tutorials/15-skill-only-enforcement.md) |
| check an endpoint, a schema, a JWT claim, a flag | [docs/reference/](docs/reference/) |
| understand *why* it is designed this way | [docs/explanation/](docs/explanation/) · [glossary](docs/explanation/glossary.md) |

## Layout

```
cmd/ppg/                 validation server (enrich, lock_in_plan, tools, verify_artifact, verify_changeset, debt_report, validate_skill, register_skill)
cmd/ppg-verify/          apply-time / CI control point: verifies the working-tree diff via /verify_changeset
cmd/svc-mock/            local stand-in for a cataloged service (runs the discovery tutorial out-of-the-box)
internal/adr/            ADR store loading + invariant retrieval
internal/enrich/         amplifier context builder
internal/catalog/        service catalog store + Rego-backed ranking (discovery)
internal/plan/           structured plan contract (see schemas/plan.schema.json)
internal/linter/         OPA/Rego plan linter, policies tagged amplifier|compensatory
internal/ticket/         capability ticket (JWT: plan_hash + scope, session-bound + configurable TTL)
internal/smarttools/     ticket guard + sandbox + semantic analyzers
internal/skill/          skill parsing + OPA/Rego governance linter + security tiers
internal/debt/           transition-debt report
internal/store/          per-machine ticket/session storage (TokenStore/SessionStore, see ADR-100)
internal/auth/           demo fixture only — frozen-legacy target for ADR-070 tutorials, not product code
internal/payment/        demo fixture only — payment router edited in the tutorials, not product code
examples/                fictional demo corpus — replace with your own (see examples/README.md)
skill-governance/        skill governance policies (structure.rego, security.rego)
schemas/                 language-neutral JSON Schema of the plan contract
adapters/preflight/      black-box adapter (writes .cursorrules / copilot-instructions.md)
adapters/claudecode/     Claude Code adapter: MCP server (planning) + PreToolUse control point
adapters/copilot/        GitHub Copilot adapter: PreToolUse control point (ppg-copilot-guard)
scripts/                 setup/remove scripts for the governed workstation
Makefile                 build, install, and setup/remove targets
demo/                    APM package: three skills (ppg-tutorial, add-payment-method, design-system)
docs/                    Diátaxis documentation + decisions (ADR-130) + PlantUML diagrams
```
