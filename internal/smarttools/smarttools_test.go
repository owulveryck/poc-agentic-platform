package smarttools

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/ticket"
)

func lockedTicket(t *testing.T) string {
	t.Helper()
	p := &plan.Plan{
		SessionID: "11111111-1111-1111-1111-111111111111",
		Intent:    "Add the Seka payment method",
		RepositoryContext: plan.RepoContext{
			Name:      "checkout-service",
			TechStack: []string{"Go"},
		},
		Steps: []plan.Step{
			{ID: "s1", Action: "edit", Tool: "patch_code", Targets: []string{"internal/payment/router.go"}},
		},
	}
	tok, err := ticket.Issue(p)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestOutOfScopeTargetIsRefusedDeterministically(t *testing.T) {
	tok := lockedTicket(t)
	_, err := Guard(tok, "patch_code", []string{"internal/auth/login.go"})
	var oos *OutOfScopeError
	if !errors.As(err, &oos) {
		t.Fatalf("expected OutOfScopeError, got %v", err)
	}
	if oos.Code != "OUT_OF_PLAN_SCOPE" {
		t.Errorf("expected OUT_OF_PLAN_SCOPE, got %s", oos.Code)
	}
}

func TestToolNotInPlanIsRefused(t *testing.T) {
	tok := lockedTicket(t)
	_, err := Guard(tok, "apply_db_migration", []string{"internal/payment/router.go"})
	var oos *OutOfScopeError
	if !errors.As(err, &oos) {
		t.Fatalf("expected OutOfScopeError, got %v", err)
	}
	if oos.Code != "TOOL_NOT_IN_PLAN" {
		t.Errorf("expected TOOL_NOT_IN_PLAN, got %s", oos.Code)
	}
}

func TestInScopeActionPassesTheGuard(t *testing.T) {
	tok := lockedTicket(t)
	claims, err := Guard(tok, "patch_code", []string{"internal/payment/router.go"})
	if err != nil {
		t.Fatalf("expected the guard to pass, got %v", err)
	}
	if claims.PlanHash == "" {
		t.Error("expected claims with a plan hash")
	}
}

// stubTool records whether it ran, so a test can assert the artifact-policy
// gate short-circuits before execution.
type stubTool struct{ ran *bool }

func (stubTool) ID() string { return "patch_code" }
func (s stubTool) Run(targets []string, payload map[string]any) map[string]any {
	*s.ran = true
	return map[string]any{"status": "OK"}
}

func TestRunEnforcesArtifactPolicy(t *testing.T) {
	ran := false
	Catalog["patch_code"] = ToolMeta{Tool: stubTool{ran: &ran}}
	t.Cleanup(func() { delete(Catalog, "patch_code") })

	// Injected evaluator flags any content containing "RAW".
	SetArtifactEvaluator(func(path, content string) []string {
		if strings.Contains(content, "RAW") {
			return []string{"raw value forbidden in " + path}
		}
		return nil
	})
	t.Cleanup(func() { SetArtifactEvaluator(nil) })

	tok := lockedTicket(t)

	out, err := Run(tok, "patch_code", []string{"internal/payment/router.go"}, map[string]any{"content": "RAW #F0F"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ran {
		t.Fatal("tool ran despite an artifact-policy violation")
	}
	if out["error_category"] != "ARCHITECTURAL_INVARIANT_VIOLATION" {
		t.Fatalf("expected ARCHITECTURAL_INVARIANT_VIOLATION, got %v", out)
	}

	ran = false
	out, err = Run(tok, "patch_code", []string{"internal/payment/router.go"}, map[string]any{"content": "clean content"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("tool did not run for clean content")
	}
	if out["status"] != "OK" {
		t.Fatalf("expected OK, got %v", out)
	}
}

func TestTargetAllowedPathBoundary(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		allowed []string
		want    bool
	}{
		{"exact file", "internal/payment/router.go", []string{"internal/payment/router.go"}, true},
		{"sibling prefix file rejected", "internal/payment/router.go.bak", []string{"internal/payment/router.go"}, false},
		{"sibling dir prefix rejected", "internal/payment_backdoor.go", []string{"internal/payment"}, false},
		{"file under allowed dir", "internal/payment/router.go", []string{"internal/payment"}, true},
		{"allowed dir itself", "internal/payment", []string{"internal/payment"}, true},
		{"wildcard dir", "internal/payment/router.go", []string{"internal/payment/*"}, true},
		{"wildcard sibling rejected", "internal/payments_secret.go", []string{"internal/payment/*"}, false},
		{"traversal escape rejected", "internal/payment/../../etc/passwd", []string{"internal/payment/*"}, false},
		{"traversal within scope allowed", "internal/payment/sub/../router.go", []string{"internal/payment"}, true},
		{"allow all", "anything/at/all.go", []string{"*"}, true},
		{"no match", "internal/auth/login.go", []string{"internal/payment"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := targetAllowed(tc.target, tc.allowed); got != tc.want {
				t.Errorf("targetAllowed(%q, %v) = %v, want %v", tc.target, tc.allowed, got, tc.want)
			}
		})
	}
}

func TestGuardRejectsSiblingPrefixTarget(t *testing.T) {
	tok := lockedTicket(t)
	// The ticket scopes exactly internal/payment/router.go; a prefix sibling
	// must not slip through the capability boundary.
	_, err := Guard(tok, "patch_code", []string{"internal/payment/router.go.bak"})
	var oos *OutOfScopeError
	if !errors.As(err, &oos) {
		t.Fatalf("expected OutOfScopeError for sibling-prefix target, got %v", err)
	}
}

func TestIsHarnessMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"plan file", filepath.Join(home, ".claude", "plans", "foo.md"), true},
		{"nested plan file", filepath.Join(home, ".claude", "plans", "sub", "bar.md"), true},
		{"harness settings stays guarded", filepath.Join(home, ".claude", "settings.json"), false},
		{"repo product file", "internal/payment/router.go", false},
		{"empty path", "", false},
		{"plans-lookalike outside home", "/tmp/.claude/plans/x.md", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsHarnessMetadata(tc.path); got != tc.want {
				t.Errorf("IsHarnessMetadata(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
