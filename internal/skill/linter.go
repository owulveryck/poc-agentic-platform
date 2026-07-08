package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Violation is a single governance finding returned when a skill fails validation.
type Violation struct {
	// Field identifies the skill field or aspect that failed (e.g. "name", "rego_policy").
	Field string `json:"field"`
	// Message is a human-readable explanation of the violation and how to fix it.
	Message string `json:"message"`
	// Nature positions the violated rule on the durability axis.
	// "amplifier" means the rule is a durable SDLC invariant;
	// "compensatory" means it is temporary scaffolding with a sunset condition.
	Nature string `json:"nature"`
}

// Linter evaluates enterprise governance policies against a skill using the
// embedded OPA engine. Policies live in a flat directory of .rego files all
// sharing package ppg.skills.governance; their violation rules union automatically.
type Linter struct {
	prepared *rego.PreparedEvalQuery
}

// NewLinter builds a Linter from all .rego files in governancePolicyDir.
// All files must belong to package ppg.skills.governance and define
// violation contains v if {...} rules.
func NewLinter(governancePolicyDir string) (*Linter, error) {
	files, err := filepath.Glob(filepath.Join(governancePolicyDir, "*.rego"))
	if err != nil {
		return nil, fmt.Errorf("listing Rego files in %s: %w", governancePolicyDir, err)
	}
	if len(files) == 0 {
		return &Linter{}, nil
	}

	ctx := context.Background()
	pq, err := rego.New(
		rego.Query("data.ppg.skills.governance.violation"),
		rego.Load(files, nil),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("preparing OPA skill governance query: %w", err)
	}
	l := &Linter{prepared: &pq}
	return l, nil
}

// Validate runs all governance policies against the skill and returns violations.
// An empty slice means the skill passes all checks and can be published.
func (l *Linter) Validate(s *Skill) []Violation {
	if l.prepared == nil {
		return nil
	}

	ctx := context.Background()
	rs, err := l.prepared.Eval(ctx, rego.EvalInput(s))
	if err != nil {
		return []Violation{{
			Field:   "linter",
			Message: fmt.Sprintf("OPA evaluation error: %v", err),
			Nature:  "compensatory",
		}}
	}
	if len(rs) == 0 || rs[0].Expressions[0].Value == nil {
		return nil
	}

	// Fail closed: a skill whose evaluation result cannot be decoded must be
	// rejected, not silently published.
	raw, err := json.Marshal(rs[0].Expressions[0].Value)
	if err != nil {
		return []Violation{{
			Field:   "linter",
			Message: fmt.Sprintf("cannot encode OPA result: %v", err),
			Nature:  "compensatory",
		}}
	}
	var violations []Violation
	if err := json.Unmarshal(raw, &violations); err != nil {
		return []Violation{{
			Field:   "linter",
			Message: fmt.Sprintf("cannot decode OPA violations: %v", err),
			Nature:  "compensatory",
		}}
	}
	return violations
}

// Tier returns the security tier of the skill based on the tools it instructs
// the agent to use. Tier 0 = read-only, Tier 1 = file modifications, Tier 2 = shell.
// This is computed in Go rather than Rego to keep the violation rules focused on
// structural and semantic governance rather than classification.
func (l *Linter) Tier(s *Skill) int {
	if strings.Contains(s.Body, "Bash") {
		return 2
	}
	if strings.Contains(s.Body, "Edit") || strings.Contains(s.Body, "Write") {
		return 1
	}
	return 0
}
