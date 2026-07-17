package linter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

// skillsDir builds a published-skills directory with one tier-1 skill
// (companion Rego requiring a design/tokens.css step for UI targets) and one
// tier-0 skill without a companion.
func skillsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	ds := filepath.Join(dir, "design-system")
	if err := os.MkdirAll(ds, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: design-system\ndescription: test\nversion: 1.0.0\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(ds, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	rego := `package ppg.skills.design_system

import rego.v1

violation contains v if {
	some step in input.steps
	endswith(step.targets[_], ".css")
	not plan_reads_design_tokens
	v := {
		"policy_id": "design_tokens_referenced",
		"message":   "plan touches a UI file but no step reads design/tokens.css",
		"nature":    "amplifier",
	}
}

plan_reads_design_tokens if {
	some step in input.steps
	step.targets[_] == "design/tokens.css"
}
`
	if err := os.WriteFile(filepath.Join(ds, "SKILL.rego"), []byte(rego), 0o644); err != nil {
		t.Fatal(err)
	}

	t0 := filepath.Join(dir, "tier0-skill")
	if err := os.MkdirAll(t0, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(t0, "SKILL.md"), []byte("---\nname: tier0-skill\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func gate3Plan(skillID string, targets ...string) *plan.Plan {
	steps := make([]plan.Step, len(targets))
	for i, target := range targets {
		steps[i] = plan.Step{ID: string(rune('a' + i)), Action: "edit", Tool: "patch_code", Targets: []string{target}}
	}
	return &plan.Plan{
		SessionID:         "s",
		SkillID:           skillID,
		Intent:            "style the landing page",
		RepositoryContext: plan.RepoContext{Name: "svc", TechStack: []string{"CSS"}},
		Steps:             steps,
	}
}

func gate3Linter(t *testing.T) *Linter {
	t.Helper()
	l, err := New(&adr.Store{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.LoadSkillCompanions(skillsDir(t)); err != nil {
		t.Fatal(err)
	}
	if l.SkillCount() != 2 {
		t.Fatalf("SkillCount = %d, want 2", l.SkillCount())
	}
	return l
}

func TestGate3CompanionRejectsNonConformingPlan(t *testing.T) {
	l := gate3Linter(t)
	violations := l.Validate(gate3Plan("design-system", "site/landing.css"))
	if len(violations) != 1 || violations[0].PolicyID != "design_tokens_referenced" {
		t.Fatalf("violations = %v, want one design_tokens_referenced", violations)
	}
}

func TestGate3CompanionAcceptsConformingPlan(t *testing.T) {
	l := gate3Linter(t)
	violations := l.Validate(gate3Plan("design-system", "site/landing.css", "design/tokens.css"))
	if len(violations) != 0 {
		t.Fatalf("violations = %v, want none", violations)
	}
}

func TestGate3UnknownSkillFailsClosed(t *testing.T) {
	l := gate3Linter(t)
	violations := l.Validate(gate3Plan("no-such-skill", "a.go"))
	if len(violations) != 1 || violations[0].PolicyID != "unknown_skill" {
		t.Fatalf("violations = %v, want one unknown_skill", violations)
	}
}

func TestGate3SkillIDWithoutRegistryFailsClosed(t *testing.T) {
	l, err := New(&adr.Store{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// No LoadSkillCompanions call: the gateway was started without -skills.
	violations := l.Validate(gate3Plan("design-system", "a.go"))
	if len(violations) != 1 || violations[0].PolicyID != "unknown_skill" {
		t.Fatalf("violations = %v, want one unknown_skill", violations)
	}
}

func TestGate3Tier0SkillHasNoCompanionViolations(t *testing.T) {
	l := gate3Linter(t)
	violations := l.Validate(gate3Plan("tier0-skill", "a.go"))
	if len(violations) != 0 {
		t.Fatalf("violations = %v, want none for a companion-less skill", violations)
	}
}

func TestGate3SkilllessPlanUnaffected(t *testing.T) {
	l := gate3Linter(t)
	violations := l.Validate(gate3Plan("", "site/landing.css"))
	if len(violations) != 0 {
		t.Fatalf("violations = %v, want none without skill_id", violations)
	}
}
