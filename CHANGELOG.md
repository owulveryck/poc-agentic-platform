# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org).

## [Unreleased]

Implements the 2026-07-21 audit transformation plan (see `AUDIT.md`).

### Added

- **`POLICY_CONFLICT` livelock escalation**: 3 consecutive `lock_in_plan`
  rejections with a byte-identical violation set now return a
  hard-blocking `409` naming the clashing policies and their sources
  (`adr`/`skill`/`built-in`), and append a record to the escalation log
  (`$XDG_STATE_HOME/ppg/escalations.jsonl`).
- **Union semantics for skill content policies**: every registered skill
  companion (operator tier + session uploads) is evaluated at the
  artifact and changeset altitudes regardless of the declared `skill_id`
  — a plan that omits the skill no longer bypasses its content
  invariants. Plan-view workflow rules remain declaration-scoped.
- **Unified skill discovery**: the MCP server now scans `~/.claude/skills`
  (user-wide) and `<project>/.agents/skills` in addition to the
  project-local `.claude/skills`, project winning on a name collision.
- **Hot reload**: `SIGHUP` rebuilds the whole corpus (ADRs, policies,
  operator skills, governance, catalog) and swaps it atomically;
  fail-safe on error; session-scoped skills survive the swap.
- **Deterministic-by-construction policies**: the OPA engine is compiled
  without nondeterministic built-ins (`http.send`, `time.now_ns`, …);
  such policies now fail at compile/registration/publish time.
- **Gate 1 compiles the bundle**: `/validate_skill` compiles the
  companion `SKILL.rego` (broken, package-less, or nondeterministic
  companions are refused at publish time). Tier is now single-sourced in
  Go and consumed by `security.rego` as `input.tier`.
- **Scripts**: `setup-git-backstop.sh` (ppg-verify as a pre-commit hook,
  repo-local or machine-wide), `setup-gateway-service.sh` /
  `remove-gateway-service.sh` (launchd/systemd user service), plus
  matching Makefile targets.
- **Docs**: problem-first README with the LLM-judge comparison table,
  English glossary, ADR-130 (naming: validation server / control
  points), golden-path onboarding, "bundle validation with a skill"
  how-to.

### Changed

- `-adr` is now **optional**: `ppg` starts with skill companions and
  built-in rules only (the tutorial-15 shape); the stub ADR workaround
  is gone.
- The guards always consult the artifact policy, even for empty or
  unrecognized content payloads (path-scoped content rules apply).
- Smart Tools evaluate the artifact policy over every payload string
  field and every target, not just `content`/`statement` on the first
  target.
- `ppg-verify` includes deletions in the changeset (`op: "delete"`).
- `ppg-verify` gained `-server` (`-gateway` kept as a deprecated alias,
  ADR-130).
- The managed-scope setup script refuses (unless `FORCE=1`) when the
  resolved `ppg-guard` binary is user-writable, and supports pinning
  `PPG_URL` via a root-owned wrapper (`PPG_PIN_URL`).
- The design-system demo skill protects `design/tokens.css` at the
  artifact/changeset altitudes too, and its body no longer advises
  re-planning a write to the tokens file.
- Session skill uploads shadowed by an operator skill are now logged
  instead of silent.

## [1.0.0-alpha] - 2026-07-17

Pre-release of 1.0.0, published for testing: the PoC hardened into a
reproducible reference implementation of the amplified agentic loop. The simulated engines
(keyword ADR retrieval, staging-schema mock, go/parser sandbox) remain
PoC-scoped by design and are documented as such in `AUDIT.md`.

### Added

- **Service catalog plane**: `POST /discover_service`, `GET /services`,
  `GET /services/{id}` backed by `internal/catalog` (Markdown records +
  Rego ranker), the `find_platform_service` MCP tool, ADR-110
  (`use_cataloged_services`) enforcement at all three altitudes,
  `svc-mock` demo service, tutorial 13 and a 13-check demo harness.
- **CI**: GitHub Actions running build, vet, golangci-lint, race tests,
  and both hermetic integration harnesses (red-team bypass: 19 checks;
  service catalog: 13 checks).
- **Release machinery**: version stamped into all seven binaries via
  ldflags (`-version` flag on each), GoReleaser configuration, this
  changelog.
- **Security hardening**: JWT signing secret now loaded from
  `PPG_TICKET_SECRET` or generated per machine on first run (the
  hard-coded PoC secret is gone); gateway HTTP server timeouts and
  request-body size limits; empty-field validation on `/enrich`,
  `/verify_artifact`, `/discover_service`; deny-by-default scope-breadth
  cap on plans (a `"."`/`"*"` scope no longer yields an allow-all
  ticket); fail-closed `Plan.Hash()`.
- **Skill Gate 3**: a plan may carry `skill_id`; the plan linter unions
  the published skill's companion Rego into the evaluation.
- Go tests for `cmd/ppg-verify`, `internal/enrich`,
  `internal/smarttools/translate`.

### Changed

- The fictional demo corpus moved from `adr/` to `examples/adr/`;
  `ppg -adr` is now explicitly required (no silent default).

## [0.0.3] - 2026-07-08

- Docs restructured into Diátaxis directories (`docs/{tutorials,how-to,
  reference,explanation}/`); first conformance audit (`AUDIT.md`);
  fail-open linters fixed to fail closed; two reachable panics fixed.

## [0.0.2] - 2026-07-08

- Plan linting moved to embedded Open Policy Agent (Rego policies paired
  with ADRs, three altitudes via `input.view`).

## [0.0.1] - 2026-07-07

- Initial release: gateway (`/enrich`, `/lock_in_plan`, smart tools,
  debt report, skill governance), Claude Code and Copilot adapters,
  capability tickets, MIT license.

[Unreleased]: https://github.com/owulveryck/poc-agentic-platform/compare/v1.0.0-alpha...HEAD
[1.0.0-alpha]: https://github.com/owulveryck/poc-agentic-platform/compare/v0.0.3...v1.0.0-alpha
[0.0.3]: https://github.com/owulveryck/poc-agentic-platform/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/owulveryck/poc-agentic-platform/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/owulveryck/poc-agentic-platform/releases/tag/v0.0.1
