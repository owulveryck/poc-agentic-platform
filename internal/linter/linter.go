// Package linter is the deterministic plan validator behind lock_in_plan.
//
// It is deliberately NOT an LLM: it runs plain code over the structured plan,
// so a non-conforming plan is rejected 100% of the time, reproducibly. Each
// policy is tagged with its nature on the durability axis (amplifier vs
// compensatory) and, when compensatory, carries a measurable sunset condition.
// Production note: these rules map one-to-one to Open Policy Agent / Rego
// policies; plain Go keeps the PoC dependency-free.
package linter

import (
	"strings"

	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

// Nature positions an artifact on the durability axis.
type Nature string

const (
	// Amplifier marks an artifact as a durable asset: its value increases as
	// model capabilities improve, so it is never scheduled for removal.
	Amplifier Nature = "amplifier"
	// Compensatory marks an artifact as temporary scaffolding: it compensates
	// for a current model limitation and must carry a measurable sunset
	// condition that determines when it can be removed.
	Compensatory Nature = "compensatory"
)

// PolicyMeta is the governance record of one policy.
type PolicyMeta struct {
	// Nature positions the policy on the durability axis (Amplifier or Compensatory).
	Nature Nature `json:"nature"`
	// Rationale explains why this policy exists and what invariant it enforces.
	Rationale string `json:"rationale"`
	// SunsetCondition is the measurable condition under which a Compensatory
	// policy can be removed. Empty for Amplifier policies.
	SunsetCondition string `json:"sunset_condition,omitempty"`
}

// Registry is the tagged policy catalog. The transition-debt report is
// computed from it: the compensatory ratio must trend toward zero over time.
var Registry = map[string]PolicyMeta{
	"go_tests_present": {
		Nature:    Amplifier,
		Rationale: "SDLC invariant: the tests must exist, whoever writes them.",
	},
	"db_migration_precedes_code": {
		Nature:    Amplifier,
		Rationale: "Ordering invariant, true whatever the model.",
	},
	"external_call_via_proxy": {
		Nature:    Amplifier,
		Rationale: "Organizational security constraint, enforced declaratively via ADR-042.",
	},
	"explicit_frozen_files_enumeration": {
		Nature:          Compensatory,
		Rationale:       "Exhaustive enumeration needed as long as the model cannot infer deprecated legacy code on its own.",
		SunsetCondition: "Model honors '@deprecated' semantically on >95% of an internal benchmark.",
	},
}

// frozenPaths is the compensatory enumeration governed by
// explicit_frozen_files_enumeration (see its sunset condition).
var frozenPaths = []string{"internal/old_payment.go", "internal/auth/"}

// Violation is a semantic, actionable rejection reason returned to the agent.
type Violation struct {
	// PolicyID identifies which policy was violated (matches a key in Registry).
	PolicyID string `json:"policy_id"`
	// Message is a human-readable, agent-facing explanation of the violation
	// and what must be changed for the plan to pass.
	Message string `json:"message"`
	// Nature mirrors the nature of the violated policy so consumers can
	// distinguish durable invariants from compensatory scaffolding rejections.
	Nature Nature `json:"nature"`
}

// Validate runs every deterministic policy over the plan and returns the
// violations. An empty slice means the plan can be locked.
func Validate(p *plan.Plan) []Violation {
	var violations []Violation

	if p.HasTech("Go") && !hasGoTest(p) {
		violations = append(violations, violation("go_tests_present",
			"SDLC invariant violated: the plan must contain a 'go test' step for a Go stack."))
	}

	if modifiesDB(p) && !hasMigrationStep(p) {
		violations = append(violations, violation("db_migration_precedes_code",
			"Invalid ordering: a schema migration step (tool 'db-migration-generator') must accompany any database change."))
	}

	for _, s := range p.Steps {
		for _, t := range s.Targets {
			for _, fp := range frozenPaths {
				if strings.HasPrefix(t, fp) {
					violations = append(violations, violation("explicit_frozen_files_enumeration",
						"Frozen zone: modifying '"+t+"' is forbidden (deprecated legacy code)."))
				}
			}
		}
	}

	return violations
}

func violation(policyID, msg string) Violation {
	return Violation{PolicyID: policyID, Message: msg, Nature: Registry[policyID].Nature}
}

func hasGoTest(p *plan.Plan) bool {
	for _, s := range p.Steps {
		if s.Tool == "go-test" || strings.Contains(strings.ToLower(s.Action), "go test") {
			return true
		}
	}
	return false
}

func modifiesDB(p *plan.Plan) bool {
	for _, s := range p.Steps {
		for _, t := range s.Targets {
			if strings.Contains(strings.ToLower(t), "db/") || strings.HasSuffix(strings.ToLower(t), ".sql") {
				return true
			}
		}
	}
	return false
}

func hasMigrationStep(p *plan.Plan) bool {
	for _, s := range p.Steps {
		if s.Tool == "db-migration-generator" {
			return true
		}
	}
	return false
}
