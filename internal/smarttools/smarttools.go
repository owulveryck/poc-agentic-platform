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
	"strings"

	"github.com/owulveryck/poc-agentic-platform/internal/ticket"
)

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
	if _, err := Guard(rawTicket, toolID, targets); err != nil {
		return nil, err
	}
	return meta.Tool.Run(targets, payload), nil
}

func targetAllowed(target string, allowed []string) bool {
	for _, a := range allowed {
		if strings.HasPrefix(target, strings.TrimSuffix(a, "*")) {
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
