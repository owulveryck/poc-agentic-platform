package linter

import (
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

func TestIsWideTarget(t *testing.T) {
	wide := []string{".", "./", "/", "*", "**", "./*", "", "  ", "..", "../", "../../etc"}
	for _, target := range wide {
		if !isWideTarget(target) {
			t.Errorf("isWideTarget(%q) = false, want true", target)
		}
	}
	narrow := []string{"internal/payment", "internal/payment/*", "migrations/001_seka.sql", "a.go", "design/tokens.css"}
	for _, target := range narrow {
		if isWideTarget(target) {
			t.Errorf("isWideTarget(%q) = true, want false", target)
		}
	}
}

func TestWideScopeViolations(t *testing.T) {
	p := &plan.Plan{
		SessionID: "s",
		Intent:    "broad change",
		RepositoryContext: plan.RepoContext{
			Name: "svc", TechStack: []string{"Go"},
		},
		Steps: []plan.Step{
			{ID: "s1", Action: "edit everything", Tool: "patch_code", Targets: []string{"."}},
			{ID: "s2", Action: "edit one file", Tool: "patch_code", Targets: []string{"internal/payment/router.go"}},
		},
	}

	violations := wideScopeViolations(p)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1: %v", len(violations), violations)
	}
	if violations[0].PolicyID != "scope_breadth_cap" {
		t.Errorf("policy_id = %s, want scope_breadth_cap", violations[0].PolicyID)
	}
	if violations[0].Nature != Amplifier {
		t.Errorf("nature = %s, want amplifier", violations[0].Nature)
	}
}

// The cap is a product rule enforced by Validate unless AllowWideScope is set;
// it must not be a Registry entry (that would skew the transition-debt ratio).
func TestValidateEnforcesScopeCapUnlessAllowed(t *testing.T) {
	l, err := New(&adr.Store{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wide := &plan.Plan{
		SessionID: "s",
		Intent:    "broad change",
		RepositoryContext: plan.RepoContext{
			Name: "svc", TechStack: []string{"Go"},
		},
		Steps: []plan.Step{{ID: "s1", Action: "edit everything", Tool: "patch_code", Targets: []string{"*"}}},
	}

	if got := l.Validate(wide); len(got) != 1 || got[0].PolicyID != "scope_breadth_cap" {
		t.Fatalf("default Validate = %v, want a single scope_breadth_cap violation", got)
	}
	if _, ok := l.Registry["scope_breadth_cap"]; ok {
		t.Error("scope_breadth_cap must not be a Registry entry")
	}

	l.AllowWideScope = true
	if got := l.Validate(wide); len(got) != 0 {
		t.Errorf("AllowWideScope Validate = %v, want no violations", got)
	}
}
