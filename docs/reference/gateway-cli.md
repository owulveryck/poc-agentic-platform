# Gateway and adapter binaries

## `cmd/ppg` — the gateway

```bash
go run ./cmd/ppg [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-addr` | `:8000` | Listen address |
| `-adr` | `adr` | Path to the ADR store (Markdown + paired `.rego` files) |
| `-skill-governance` | `skill-governance` | Path to the skill governance Rego policy directory |

Startup logs three readiness lines: `ADR store loaded: N invariants`,
`Plan linter ready: N policies`, `Skill governance linter ready`, then
`Platform Planning Gateway listening on <addr>`.

## `adapters/claudecode/mcpserver` — MCP server (stdio)

Exposes `get_platform_guidelines_for_intent` and `lock_in_plan` as native
tools; writes the capability ticket to `.ppg-ticket` on a successful lock.

| Environment variable | Default | Description |
|---|---|---|
| `PPG_URL` | `http://localhost:8000` | Gateway base URL |

## `adapters/claudecode/guard` — `ppg-guard` PreToolUse hook

Reads the hook JSON from stdin, verifies the target file against the
`.ppg-ticket` scope, exits 2 with a semantic message on stderr to block the
tool call. No flags, no environment.

## `adapters/preflight` — black-box pre-flight

```bash
go run ./adapters/preflight [-repo <name>] [-stack Go,SQL] "<intent>"
```

| Flag / env | Default | Description |
|---|---|---|
| `-repo` | `checkout-service` | Sent as `repository_context.name` |
| `-stack` | `Go` | Comma-separated, sent as `repository_context.tech_stack` |
| `PPG_URL` | `http://localhost:8000` | Gateway base URL (same convention as the MCP server) |

Writes the enriched invariants to `.cursorrules` and
`.github/copilot-instructions.md` in the current directory.

## Dependencies

| Module | Role |
|---|---|
| `github.com/golang-jwt/jwt/v5` | Capability ticket signing/verification |
| `github.com/open-policy-agent/opa` | Embedded policy engine (Go library — no external OPA binary required) |
| `github.com/modelcontextprotocol/go-sdk` | MCP server for the Claude Code adapter |
| `gopkg.in/yaml.v3` | ADR and skill front-matter parsing |

Why OPA/Rego rather than plain Go policies: see
[dual-representation-adr.md](../explanation/dual-representation-adr.md#why-oparego-for-plan-enforcement).
