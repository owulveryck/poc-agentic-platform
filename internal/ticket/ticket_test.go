package ticket

import (
	"testing"

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
	if claims.PlanHash != p.Hash() {
		t.Errorf("plan hash mismatch: %s vs %s", claims.PlanHash, p.Hash())
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

func TestTamperedTicketIsRejected(t *testing.T) {
	tok, err := Issue(testPlan())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(tok + "x"); err == nil {
		t.Fatal("expected a tampered ticket to be rejected")
	}
}
