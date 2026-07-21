---
adr_id: ADR-130
title: '"Gateway" names a control point; the central service is the "validation server"'
status: accepted
nature: amplifier
sunset_condition: null
scope_selectors: ["naming", "terminology", "glossary", "gateway", "documentation"]
enforcement:
  mode: declarative
  policy_id: null
---

## Context

The project's intended vocabulary defines a **gateway** as one individual
deterministic enforcement point (a policy enforcement point); the harness
is distributed — made of several gateways, not a single frontal component.

The 2026-07-21 audit found this inverted in practice: the codebase and
documentation used "gateway" exclusively for the single central HTTP server
(`cmd/ppg`, the CLI reference, the `-gateway` flag on `ppg-verify`), while
the actual distributed enforcement points carry four other names — *guard*
(`ppg-guard`, `ppg-copilot-guard`), *hook* (PreToolUse), *gate* (skill
lifecycle 1/2/3), and *view/altitude* (plan/artifact/changeset). "Control
point" appeared nowhere in English. The audit also observed that unnamed
concepts correlated exactly with unimplemented behavior — naming drift is
design drift.

## Options considered

- **A — rename the code's usage.** "Gateway" keeps the glossary meaning
  (a distributed control point); `cmd/ppg` becomes the **validation
  server** in all prose and vocabulary.
- **B — rename the concept.** Accept that "gateway" means the central
  server; keep guard/hook/gate for enforcement points; amend the glossary.
- **Defer** to a later naming rework, using neutral wording meanwhile.

## Decision

**Option A.** The distributed framing is a deliberate thesis point of the
project and is worth more than the rename costs. The architecture is
honestly described as **distributed enforcement, centralized decision**:
control points (guards, hooks, the apply-time backstop) are distributed
across the loop; they all delegate policy evaluation to one validation
server, so every control point renders identical verdicts from a single
policy corpus.

Terminology from this ADR on:

| Term | Meaning |
|---|---|
| governance harness | the whole machine-level system: control points + MCP servers + validation server |
| control point (gateway) | one deterministic enforcement point: `ppg-guard`, `ppg-copilot-guard`, the in-tool ticket check, `ppg-verify` |
| validation server | the central policy evaluator, `cmd/ppg` |

## Consequences

1. Documentation prose: "the gateway" → "the validation server" when
   referring to `cmd/ppg`; "control point" for enforcement points. New
   documents (README, glossary) are written this way from day one.
2. `docs/reference/gateway-cli.md` → `validation-server-cli.md`, keeping
   a redirect stub at the old path (same convention as `docs/tutorial.md`).
3. `ppg-verify`'s `-gateway` flag gains a `-server` alias; `-gateway`
   is kept as a deprecated synonym until v2 (no breaking change in 1.x).
4. Binary names (`ppg`, `ppg-guard`, …) and HTTP/MCP tool names are
   unchanged — the rename is vocabulary, not wire protocol.
5. The published blog articles are not retro-edited; the
   [glossary](../explanation/glossary.md) notes that article-era prose
   may use "gateway" in the old sense.

## What we do NOT write here

No file-by-file rename list. The naming-alignment task executes this
decision; this ADR only fixes the target vocabulary so that documents
written from now on stop deepening the drift.
