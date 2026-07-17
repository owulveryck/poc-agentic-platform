package ticket

import (
	"testing"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

func testPlan() *plan.Plan {
	return &plan.Plan{
		SessionID: "11111111-1111-1111-1111-111111111111",
		Intent:    "Add the Seka payment method",
		RepositoryContext: plan.RepoContext{
			Name:      "checkout-service",
			TechStack: []string{"Go"},
		},
		Steps: []plan.Step{
			{ID: "s1", Action: "edit", Tool: "patch_code", Targets: []string{"internal/payment/router.go"}},
			{ID: "s2", Action: "go test", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
		},
	}
}

func TestIssueAndVerifyRoundtrip(t *testing.T) {
	p := testPlan()
	tok, err := Issue(p)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := p.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if claims.PlanHash != wantHash {
		t.Errorf("plan hash mismatch: %s vs %s", claims.PlanHash, wantHash)
	}
	if claims.SessionID != p.SessionID {
		t.Errorf("session mismatch")
	}
}

func TestScopeIsDerivedFromSteps(t *testing.T) {
	scope := DeriveScope(testPlan())
	wantTools := map[string]bool{"patch_code": true, "go-test": true}
	for _, tool := range scope.AllowTool {
		if !wantTools[tool] {
			t.Errorf("unexpected tool in scope: %s", tool)
		}
		delete(wantTools, tool)
	}
	if len(wantTools) != 0 {
		t.Errorf("missing tools in scope: %v", wantTools)
	}
	if len(scope.AllowModify) != 2 {
		t.Errorf("expected 2 allowed files, got %v", scope.AllowModify)
	}
}

func TestExpiredTicketIsRejected(t *testing.T) {
	// JWT exp has second precision; a 1s TTL slept just past reliably expires.
	tok, err := IssueWithTTL(testPlan(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := Verify(tok); err == nil {
		t.Fatal("expected an expired ticket to be rejected")
	}
}

func TestCustomTTLIsApplied(t *testing.T) {
	tok, err := IssueWithTTL(testPlan(), 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if got := claims.ExpiresAt.Sub(claims.IssuedAt.Time); got != 2*time.Hour {
		t.Errorf("expected a 2h lifetime, got %s", got)
	}
}

func TestIssueWithTTLZeroFallsBackToDefault(t *testing.T) {
	tok, err := IssueWithTTL(testPlan(), 0)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := Verify(tok)
	if err != nil {
		t.Fatalf("a zero ttl should fall back to DefaultTTL and verify, got %v", err)
	}
	got := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if got != DefaultTTL {
		t.Errorf("expected DefaultTTL %s, got %s", DefaultTTL, got)
	}
}

func TestTamperedTicketIsRejected(t *testing.T) {
	tok, err := Issue(testPlan())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(tok + "x"); err == nil {
		t.Fatal("expected a tampered ticket to be rejected")
	}
}
