package main

import (
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

func hookPayload(filePath string) []byte {
	return []byte(`{"tool_name":"Edit","cwd":"/work/checkout-service","tool_input":{"file_path":"` + filePath + `"}}`)
}

func TestInScopeEditPasses(t *testing.T) {
	block, msg := decide(hookPayload("/work/checkout-service/internal/payment/router.go"), lockedTicket(t))
	if block {
		t.Fatalf("expected the edit to pass, got block with %q", msg)
	}
}

func TestOutOfScopeEditIsBlockedWithSemanticMessage(t *testing.T) {
	block, msg := decide(hookPayload("/work/checkout-service/internal/auth/login.go"), lockedTicket(t))
	if !block {
		t.Fatal("expected an out-of-scope edit to be blocked")
	}
	if !strings.Contains(msg, "OUT_OF_PLAN_SCOPE") || !strings.Contains(msg, "internal/auth/login.go") {
		t.Errorf("message should be semantic and name the file, got %q", msg)
	}
	if !strings.Contains(msg, "lock_in_plan") {
		t.Errorf("message should guide the agent back to lock_in_plan, got %q", msg)
	}
}

func TestMissingTicketBlocksWithGuidance(t *testing.T) {
	block, msg := decide(hookPayload("/work/checkout-service/internal/payment/router.go"), "")
	if !block {
		t.Fatal("expected a missing ticket to block")
	}
	if !strings.Contains(msg, ".ppg-ticket") {
		t.Errorf("message should mention the ticket file, got %q", msg)
	}
}

func TestNonFileToolIsIgnored(t *testing.T) {
	block, _ := decide([]byte(`{"tool_name":"Bash","cwd":"/work","tool_input":{"command":"ls"}}`), "")
	if block {
		t.Fatal("a tool call without file_path must not be blocked by the guard")
	}
}
