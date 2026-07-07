package smarttools

import (
	"errors"
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
