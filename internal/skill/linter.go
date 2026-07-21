package skill

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/owulveryck/poc-agentic-platform/internal/policy"
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
	eval *policy.Evaluator
}

// NewLinter builds a Linter from all .rego files in governancePolicyDir.
// All files must belong to package ppg.skills.governance and define
// violation contains v if {...} rules.
func NewLinter(governancePolicyDir string) (*Linter, error) {
	files, err := filepath.Glob(filepath.Join(governancePolicyDir, "*.rego"))
	if err != nil {
		return nil, fmt.Errorf("listing Rego files in %s: %w", governancePolicyDir, err)
	}
	eval, err := policy.Prepare("data.ppg.skills.governance.violation", files)
	if err != nil {
		return nil, err
	}
	return &Linter{eval: eval}, nil
}

// validationInput is the policy input document: the skill's fields plus the
// Go-computed security tier, so the Rego rules consume one source of tier
// truth (Linter.Tier) instead of re-deriving it from body keywords.
type validationInput struct {
	*Skill
	Tier int `json:"tier"`
}

// Validate runs all governance policies against the skill and returns violations.
// An empty slice means the skill passes all checks and can be published. It
// fails closed: an evaluation or decode error surfaces as a rejection. When
// the skill bundles a companion SKILL.rego, Validate also compiles it (with
// the same deterministic engine the gateway uses), so a broken or
// nondeterministic companion is refused at publish time (Gate 1) instead of
// surfacing at gateway startup or /register_skill.
func (l *Linter) Validate(s *Skill) []Violation {
	violations, err := policy.Eval[Violation](l.eval, validationInput{Skill: s, Tier: l.Tier(s)})
	if err != nil {
		return []Violation{{
			Field:   "linter",
			Message: err.Error(),
			Nature:  "compensatory",
		}}
	}
	return append(violations, compileCompanion(s)...)
}

// compileCompanion compiles the bundled SKILL.rego, when present.
func compileCompanion(s *Skill) []Violation {
	if s.RegoPolicy == "" {
		return nil
	}
	pkg, ok := scanRegoPackage(s.RegoPolicy)
	if !ok {
		return []Violation{{
			Field:   "rego_policy",
			Message: "companion SKILL.rego has no package declaration (expected e.g. `package ppg.skills.<name>`)",
			Nature:  "amplifier",
		}}
	}
	if _, err := policy.PrepareModule("data."+pkg+".violation", "SKILL.rego", s.RegoPolicy); err != nil {
		return []Violation{{
			Field:   "rego_policy",
			Message: fmt.Sprintf("companion SKILL.rego does not compile: %v", err),
			Nature:  "amplifier",
		}}
	}
	return nil
}

// scanRegoPackage returns the package path declared by a rego source (same
// shape as internal/linter's parser).
func scanRegoPackage(source string) (string, bool) {
	for line := range strings.SplitSeq(source, "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "package "); ok {
			return strings.TrimSpace(after), true
		}
	}
	return "", false
}

// Tier returns the security tier of the skill based on the tools it instructs
// the agent to use. Tier 0 = read-only, Tier 1 = file modifications, Tier 2 = shell.
// This is the SINGLE source of tier truth: the governance policies receive it
// as input.tier (see validationInput) instead of re-deriving it from body
// keywords, so the Go and Rego views of "privileged" can never drift.
func (l *Linter) Tier(s *Skill) int {
	if strings.Contains(s.Body, "Bash") {
		return 2
	}
	if strings.Contains(s.Body, "Edit") || strings.Contains(s.Body, "Write") {
		return 1
	}
	return 0
}
