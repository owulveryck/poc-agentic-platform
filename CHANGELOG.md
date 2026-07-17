# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org).

## [Unreleased]

## [1.0.0] - 2026-07-17

First stable release: the PoC hardened into a reproducible reference
implementation of the amplified agentic loop. The simulated engines
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

[Unreleased]: https://github.com/owulveryck/poc-agentic-platform/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/owulveryck/poc-agentic-platform/compare/v0.0.3...v1.0.0
[0.0.3]: https://github.com/owulveryck/poc-agentic-platform/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/owulveryck/poc-agentic-platform/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/owulveryck/poc-agentic-platform/releases/tag/v0.0.1
