// Package linter is the deterministic plan validator behind lock_in_plan.
//
// It is deliberately NOT an LLM: it evaluates Open Policy Agent (OPA/Rego)
// policies loaded from ADR-paired .rego files, so a non-conforming plan is
// rejected 100% of the time, reproducibly. Each policy is tagged with its
// nature on the durability axis (amplifier vs compensatory) and, when
// compensatory, carries a measurable sunset condition.
//
// Each ADR is a dual-representation governance artifact: the semantic
// directive (InvariantText) is injected at enrich() time to shape planning;
// the paired .rego file is evaluated at lock_in_plan time for deterministic
// enforcement. The two representations can have different lifetimes on the
// durability axis.
package linter

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/owulveryck/poc-agentic-platform/internal/adr"
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

// Linter evaluates OPA/Rego policies derived from ADR .rego files.
type Linter struct {
	// Registry is the governance catalog of all tracked policies, keyed by
	// policy_id. Used by the debt report to measure the compensatory ratio.
	Registry map[string]PolicyMeta
	prepared *rego.PreparedEvalQuery
}

// New builds a Linter from the ADR store. It populates the Registry from ADR
// metadata and compiles a single OPA PreparedEvalQuery over all paired .rego
// files found in adrDir. ADRs without a RegoFile (e.g. declarative-only ADRs)
// still contribute to the Registry but not to the Rego evaluation.
func New(store *adr.Store, adrDir string) (*Linter, error) {
	l := &Linter{Registry: make(map[string]PolicyMeta)}

	var regoPaths []string
	for _, inv := range store.Invariants {
		if inv.Enforcement.PolicyID == "" {
			continue
		}
		l.Registry[inv.Enforcement.PolicyID] = PolicyMeta{
			Nature:          Nature(inv.Nature),
			Rationale:       inv.Title,
			SunsetCondition: inv.SunsetCondition,
		}
		if inv.Enforcement.RegoFile != "" {
			regoPaths = append(regoPaths, filepath.Join(adrDir, inv.Enforcement.RegoFile))
		}
	}

	if len(regoPaths) == 0 {
		return l, nil
	}

	ctx := context.Background()
	pq, err := rego.New(
		rego.Query("data.ppg.linter.violation"),
		rego.Load(regoPaths, nil),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("preparing OPA query: %w", err)
	}
	l.prepared = &pq
	return l, nil
}

// Validate evaluates all Rego policies against the plan and returns the
// violations. An empty slice means the plan can be locked.
func (l *Linter) Validate(p *plan.Plan) []Violation {
	if l.prepared == nil {
		return nil
	}

	ctx := context.Background()
	rs, err := l.prepared.Eval(ctx, rego.EvalInput(p))
	if err != nil {
		return []Violation{{
			PolicyID: "linter_eval_error",
			Message:  fmt.Sprintf("OPA evaluation error: %v", err),
			Nature:   Compensatory,
		}}
	}
	if len(rs) == 0 || rs[0].Expressions[0].Value == nil {
		return nil
	}

	raw, err := json.Marshal(rs[0].Expressions[0].Value)
	if err != nil {
		return nil
	}
	var violations []Violation
	if err := json.Unmarshal(raw, &violations); err != nil {
		return nil
	}
	return violations
}
