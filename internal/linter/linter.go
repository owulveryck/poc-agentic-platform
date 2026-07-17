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
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/policy"
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

// Linter evaluates OPA/Rego policies derived from ADR .rego files. The same
// compiled corpus is evaluated at three altitudes — plan, artifact and
// changeset — discriminated by the input.view field (see Validate,
// EvaluateArtifact, EvaluateChangeset).
type Linter struct {
	// Registry is the governance catalog of all tracked policies, keyed by
	// policy_id. Used by the debt report to measure the compensatory ratio.
	Registry map[string]PolicyMeta
	// AllowWideScope disables the built-in scope-breadth cap: when false (the
	// default), a plan step targeting the repository root ("." / "/" / "*")
	// is rejected at lock time, because the derived capability ticket would
	// be allow-all and least privilege would be meaningless.
	AllowWideScope bool
	eval           *policy.Evaluator
}

// Artifact is one edited file's actual content — the artifact view of the
// policy input, used by the in-loop guard hook and the Smart Tools.
type Artifact struct {
	// Path is the file path being written, relative to the project root.
	Path string `json:"path"`
	// Content is the full proposed content of the file after the edit.
	Content string `json:"content"`
	// Op is the operation ("write", "edit", "create"); optional.
	Op string `json:"op,omitempty"`
}

// Changeset is a set of edited files — the changeset (diff) view of the policy
// input, used by the apply-time gate. PlanHash lets the gate detect plan
// substitution against the ticket.
type Changeset struct {
	// Files are the changed files with their post-change content.
	Files []Artifact `json:"files"`
	// PlanHash is the fingerprint of the plan the changeset claims to execute.
	PlanHash string `json:"plan_hash,omitempty"`
}

// planInput is the plan view: the plan fields promoted to the top level (so
// existing rules reading input.steps keep working) plus the view discriminator.
type planInput struct {
	plan.Plan
	View string `json:"view"`
}

type artifactInput struct {
	View     string   `json:"view"`
	Artifact Artifact `json:"artifact"`
}

type changesetInput struct {
	View      string    `json:"view"`
	Changeset Changeset `json:"changeset"`
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

	eval, err := policy.Prepare("data.ppg.linter.violation", regoPaths)
	if err != nil {
		return nil, err
	}
	l.eval = eval
	return l, nil
}

// Validate evaluates all Rego policies against the plan (plan view) and returns
// the violations. An empty slice means the plan can be locked. Unless
// AllowWideScope is set, it also enforces the built-in scope-breadth cap —
// a product rule rather than an ADR policy, so it is not part of Registry.
func (l *Linter) Validate(p *plan.Plan) []Violation {
	var violations []Violation
	if !l.AllowWideScope {
		violations = wideScopeViolations(p)
	}
	return append(violations, l.evaluate(planInput{Plan: *p, View: "plan"})...)
}

// wideScopeViolations rejects step targets so broad that the ticket derived
// from them would be allow-all (deny-by-default cap on scope breadth).
func wideScopeViolations(p *plan.Plan) []Violation {
	var violations []Violation
	for _, s := range p.Steps {
		for _, t := range s.Targets {
			if isWideTarget(t) {
				violations = append(violations, Violation{
					PolicyID: "scope_breadth_cap",
					Message: fmt.Sprintf("step %q: target %q is too broad — the derived ticket would allow modifying the whole repository. "+
						"Enumerate the files or directories the step actually touches (operators can restore the old behavior with ppg -allow-wide-scope).",
						s.ID, t),
					Nature: Amplifier,
				})
			}
		}
	}
	return violations
}

// isWideTarget reports whether a target grants an effectively unlimited file
// scope. A trailing "*" is a prefix pattern in the ticket scope, so the check
// applies to the prefix that remains once wildcards are stripped.
func isWideTarget(target string) bool {
	t := strings.TrimSpace(target)
	if t == "" {
		return true
	}
	prefix := strings.TrimRight(t, "*")
	if prefix == "" { // "*", "**"
		return true
	}
	clean := path.Clean(prefix)
	if clean == "." || clean == ".." || clean == "/" {
		return true
	}
	return strings.HasPrefix(clean, "../")
}

// EvaluateArtifact evaluates the corpus against a single edited file's content
// (artifact view) — the in-loop check behind the guard hook and Smart Tools.
func (l *Linter) EvaluateArtifact(a Artifact) []Violation {
	return l.evaluate(artifactInput{View: "artifact", Artifact: a})
}

// EvaluateChangeset evaluates the corpus against a whole diff (changeset view) —
// the apply-time backstop.
func (l *Linter) EvaluateChangeset(c Changeset) []Violation {
	return l.evaluate(changesetInput{View: "changeset", Changeset: c})
}

// evaluate runs the corpus against one input document. It fails closed: an
// evaluation or decode error surfaces as a synthetic rejection rather than a
// silent pass.
func (l *Linter) evaluate(input any) []Violation {
	violations, err := policy.Eval[Violation](l.eval, input)
	if err != nil {
		return []Violation{{
			PolicyID: "linter_eval_error",
			Message:  err.Error(),
			Nature:   Compensatory,
		}}
	}
	return violations
}
