// Package smarttools implements the Smart Platform Tool contract:
//
//  1. VerifyScope — check the capability ticket in-tool (least privilege);
//  2. ExecuteSandbox — act on an isolated copy, never directly on the target;
//  3. Analyze — turn the raw outcome into semantic, actionable feedback.
//
// A Smart Tool is a deterministic mentor, not a passive executor returning
// "exit 1".
package smarttools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/translate"
	"github.com/owulveryck/poc-agentic-platform/internal/ticket"
)

// ArtifactEvaluator reports the architectural-invariant violations of a file's
// proposed content (the artifact-view policy). It is injected by the gateway to
// keep this package decoupled from the linter; an empty return means the content
// is clean. When nil (unwired), Run skips the content check. The skillID and
// sessionID are taken from the ticket claims: skillID selects the skill
// companion, sessionID selects the session-scoped tier (client-uploaded skills)
// when the operator-provided tier has no entry for that name.
type ArtifactEvaluator func(path, content, skillID, sessionID string) []string

var artifactEvaluator ArtifactEvaluator

// SetArtifactEvaluator wires the artifact-view policy evaluator used by Run so a
// Smart Tool refuses content that breaks an invariant, not just out-of-scope
// paths. Wired once at startup.
func SetArtifactEvaluator(fn ArtifactEvaluator) { artifactEvaluator = fn }

// contentKeys are the payload fields a Smart Tool may carry file/artifact
// content under, in priority order.
var contentKeys = []string{"content", "statement"}

// OutOfScopeError is the deterministic refusal returned when the agent drifts
// from its locked plan. Nothing has been executed when it is raised.
type OutOfScopeError struct {
	// Code is the machine-readable refusal category:
	// TOOL_NOT_IN_PLAN means the tool ID is absent from the ticket scope;
	// OUT_OF_PLAN_SCOPE means one or more target paths are not allowed.
	Code string `json:"code"`
	// Attempted is the tool ID or file path that triggered the refusal.
	Attempted string `json:"attempted"`
	// Allowed is the set of tool IDs or file paths the ticket does permit.
	Allowed []string `json:"allowed"`
}

// Error formats the refusal as "CODE: attempted "X", allowed [Y Z]".
func (e *OutOfScopeError) Error() string {
	return fmt.Sprintf("%s: attempted %q, allowed %v", e.Code, e.Attempted, e.Allowed)
}

// Tool is the contract every Smart Platform Tool implements.
type Tool interface {
	// ID returns the unique identifier used for catalog registration and
	// ticket scope matching (e.g. "patch_code", "apply_db_migration").
	ID() string
	// Run assumes the scope has already been verified by Guard.
	Run(targets []string, payload map[string]any) map[string]any
}

// ToolMeta tags each tool on the durability axis for the debt report.
type ToolMeta struct {
	// Tool is the registered Smart Tool implementation.
	Tool Tool
	// Nature is the durability classification ("amplifier" or "compensatory").
	Nature string
	// SunsetCondition is the measurable condition under which a compensatory
	// tool can be removed. Empty for amplifier tools.
	SunsetCondition string
}

// Catalog is the registry of available Smart Tools, populated by Register.
var Catalog = map[string]ToolMeta{}

// Register adds a tool to the catalog.
func Register(t Tool, nature, sunset string) {
	Catalog[t.ID()] = ToolMeta{Tool: t, Nature: nature, SunsetCondition: sunset}
}

// Guard verifies the capability ticket against the requested tool and
// targets. It is called at the entry of EVERY tool: agentic drift happens
// during execution, so the last deterministic line of defense lives in-tool.
func Guard(rawTicket, toolID string, targets []string) (*ticket.Claims, error) {
	claims, err := GuardTargets(rawTicket, targets)
	if err != nil {
		return nil, err
	}
	if !contains(claims.Scope.AllowTool, toolID) {
		return nil, &OutOfScopeError{
			Code:      "TOOL_NOT_IN_PLAN",
			Attempted: toolID,
			Allowed:   claims.Scope.AllowTool,
		}
	}
	return claims, nil
}

// GuardTargets verifies the capability ticket against target files only —
// the check used by external agent harnesses (e.g. a Claude Code PreToolUse
// hook guarding Edit/Write calls) where the tool identity belongs to the
// harness, not to the platform catalog.
func GuardTargets(rawTicket string, targets []string) (*ticket.Claims, error) {
	claims, err := ticket.Verify(rawTicket)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired ticket: %w", err)
	}
	for _, t := range targets {
		if !targetAllowed(t, claims.Scope.AllowModify) {
			return nil, &OutOfScopeError{
				Code:      "OUT_OF_PLAN_SCOPE",
				Attempted: t,
				Allowed:   claims.Scope.AllowModify,
			}
		}
	}
	return claims, nil
}

// Run guards then executes a cataloged tool.
func Run(rawTicket, toolID string, targets []string, payload map[string]any) (map[string]any, error) {
	meta, ok := Catalog[toolID]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", toolID)
	}
	claims, err := Guard(rawTicket, toolID, targets)
	if err != nil {
		return nil, err
	}
	// Artifact-view policy: the tool holds the actual content, so it enforces
	// the invariant against what will be written, not just the path scope.
	if msgs := evaluateArtifactPolicy(targets, payload, claims.SkillID, claims.SessionID); len(msgs) > 0 {
		return translate.PolicyViolation(translate.Generic(1, "content violates architectural invariants"), msgs), nil
	}
	return meta.Tool.Run(targets, payload), nil
}

// evaluateArtifactPolicy runs the injected artifact evaluator over every
// string field of the payload, attributed to every target: content must
// satisfy the invariants whichever key it travels under and whichever
// target it lands on — a payload key the tool contract didn't anticipate is
// not a policy exemption. The preferred contentKeys are evaluated first,
// then the remaining string fields in sorted-key order, so verdict order is
// deterministic. Returns nil when no evaluator is wired or all clean.
func evaluateArtifactPolicy(targets []string, payload map[string]any, skillID, sessionID string) []string {
	if artifactEvaluator == nil || len(targets) == 0 {
		return nil
	}
	var contents []string
	seen := make(map[string]bool, len(contentKeys))
	for _, k := range contentKeys {
		if s, ok := payload[k].(string); ok && s != "" {
			contents = append(contents, s)
		}
		seen[k] = true
	}
	extra := make([]string, 0, len(payload))
	for k := range payload {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		if s, ok := payload[k].(string); ok && s != "" {
			contents = append(contents, s)
		}
	}
	var msgs []string
	dedupe := map[string]bool{}
	for _, target := range targets {
		for _, content := range contents {
			for _, m := range artifactEvaluator(target, content, skillID, sessionID) {
				if !dedupe[m] {
					dedupe[m] = true
					msgs = append(msgs, m)
				}
			}
		}
	}
	return msgs
}

// harnessMetadataSubdirs lists the home-relative directories an agent
// harness uses for its own bookkeeping — never product code. Currently just
// the plan files a harness writes during plan mode (~/.claude/plans/). Add
// new harness scratch locations here as they appear.
var harnessMetadataSubdirs = []string{filepath.Join(".claude", "plans")}

// IsHarnessMetadata reports whether filePath is agent-harness bookkeeping the
// guard must never govern — e.g. the plan files a harness writes under
// ~/.claude/plans/ during plan mode. These are harness scratch, not product
// edits, so they fall outside any capability ticket. The check is deliberately
// narrow: sibling files such as ~/.claude/settings.json stay guarded so an
// agent cannot disable the guard by editing its own hook config.
func IsHarnessMetadata(filePath string) bool {
	if filePath == "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	clean := filepath.Clean(filePath)
	for _, sub := range harnessMetadataSubdirs {
		root := filepath.Join(home, sub)
		rel, err := filepath.Rel(root, clean)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// targetAllowed reports whether target falls within one of the allowed scope
// entries. Matching is path-segment aware, not a raw string prefix: an entry
// "internal/payment" grants "internal/payment" and anything under
// "internal/payment/", but NOT the sibling "internal/payment_backdoor.go". A
// trailing "*" entry ("internal/payment/*") behaves the same as the bare
// directory. Both sides are filepath.Clean'd first, so "..\/" traversal in the
// target cannot escape its cleaned form (e.g. "internal/payment/../../etc" is
// normalized before comparison).
func targetAllowed(target string, allowed []string) bool {
	ct := filepath.Clean(target)
	for _, a := range allowed {
		base := filepath.Clean(strings.TrimSuffix(a, "*"))
		if base == "." {
			// Entry was "*" (or "./*"): an explicit allow-all scope.
			return true
		}
		if ct == base {
			return true
		}
		if strings.HasPrefix(ct, base+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
