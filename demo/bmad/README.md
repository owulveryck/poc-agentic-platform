# BMAD ↔ ppg — a live governed demo

Is the ppg governance harness useful for [BMAD](https://github.com/bmad-code-org/BMAD-METHOD)
and other spec-driven agentic methods? **Yes — for the mechanical, structural
part, plus observation; not for the judgment or the multi-agent orchestration.**
This directory demonstrates exactly where the line falls, live and reproducibly.

The failure mode these methods share: their process is a set of **directives in
the model's context** that the model can skip while *claiming* it complied — a
story with no acceptance criteria, a plan that never read the story, a Dev that
edits out of scope. The prose said so; nothing checked. ppg makes the checkable
part deterministic.

> **Pure usage, no core changes.** This is `-adr demo/bmad/adr` — two `.rego`
> policies loaded exactly like `examples/adr/*` and `demo/skills/*/SKILL.rego`.
> **No ppg core `.go` is modified and none needs to be**; ppg is agent-agnostic
> and its three policy views are treated as correct-by-design.

## Contents

| File | What it is |
|---|---|
| [`BMAD-COMPAT.md`](BMAD-COMPAT.md) | The analysis. Where the three ppg views bite the BMAD cycle, the honest boundary (judgment + orchestration stay out), amplifier vs compensatory, the observation pillar. Two Mermaid diagrams. |
| [`LIVE-DEMO.md`](LIVE-DEMO.md) | The presenter's four-Act walkthrough: install real BMAD, toggle the gateway, watch the drift ship (OFF) then get blocked (ON). Authentic refusal transcripts embedded. |
| [`run-bmad-tests.sh`](run-bmad-tests.sh) | The reproducible driver. Throwaway server on `demo/bmad/adr`, 10 assertions, each shown WITHOUT vs WITH the harness. Mutates no global config. |
| [`run-live-demo.sh`](run-live-demo.sh) | The narrated **headless** demo: the four Acts of `LIVE-DEMO.md` driven by a real Claude Code agent in `claude -p` mode. The gateway toggle is two project-local files in a throwaway dir; global Claude config untouched. Acts 1–2 outcomes are statistical (that's the thesis); Act 4's out-of-scope block is hard-asserted. |
| [`adr/`](adr/) | ADR-210 (story carries Acceptance Criteria + Tasks — *artifact/changeset*, compensatory) and ADR-211 (dev plan reads the story before writing `src/` — *plan*, amplifier), each with its `.rego`. |
| [`fixtures/`](fixtures/) | An authentic-shape BMAD story (good + AC-less drift), the in-scope impl, and the out-of-scope edit. |
| [`payloads/`](payloads/) | `/lock_in_plan` and `/enrich` request bodies used by the driver. |

## Quick start

```bash
# 1. The corpus loads (a .rego that doesn't compile => the server won't boot):
go run ./cmd/ppg -adr demo/bmad/adr -addr 127.0.0.1:8799
#   ADR store loaded: 2 invariants
#   Plan linter ready: 2 policies

# 2. The deterministic proof — 10 assertions, each WITHOUT vs WITH the harness:
bash demo/bmad/run-bmad-tests.sh
#   Summary: 10 passed, 0 failed

# 3. The narrated headless demo — the four Acts with a real (small) model,
#    non-interactive; needs an authenticated `claude` and `make install`:
bash demo/bmad/run-live-demo.sh          # AUTO=1 for no pauses, KEEP=1 to inspect
```

For the live, human-in-the-loop version (real BMAD in Claude Code, gateway
toggled between Acts), follow [`LIVE-DEMO.md`](LIVE-DEMO.md).

## The three drifts and how ppg refuses each

| BMAD drift | ppg view | Rule | Refusal |
|---|---|---|---|
| Story handed to Dev with no Acceptance Criteria / Tasks | `artifact` (+ `changeset`) | ADR-210 `bmad_story_schema_complete` (compensatory) | `ARCHITECTURAL_INVARIANT_VIOLATION` / 422 |
| Dev plan writes `src/` without reading the story | `plan` | ADR-211 `bmad_plan_references_story` (amplifier) | `PLAN_REJECTED` / 422 |
| Dev edits a file outside the story's scope | ticket scope (built-in) | capability ticket | `OUT_OF_PLAN_SCOPE` / 403 |

Full rationale, the honest boundary, and the diagrams are in
[`BMAD-COMPAT.md`](BMAD-COMPAT.md).
