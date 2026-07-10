package main

import (
	"os"
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

func hookPayload(filePath string) []byte {
	return []byte(`{"tool_name":"Edit","cwd":"/work/checkout-service","tool_input":{"file_path":"` + filePath + `"}}`)
}

func hookPayloadWithSession(filePath, sessionID string) []byte {
	return []byte(`{"tool_name":"Edit","session_id":"` + sessionID + `","cwd":"/work/checkout-service","tool_input":{"file_path":"` + filePath + `"}}`)
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

func TestMatchingSessionPasses(t *testing.T) {
	payload := hookPayloadWithSession("/work/checkout-service/internal/payment/router.go",
		"11111111-1111-1111-1111-111111111111")
	block, msg := decide(payload, lockedTicket(t))
	if block {
		t.Fatalf("expected the matching session to pass, got block with %q", msg)
	}
}

func TestSessionMismatchIsBlocked(t *testing.T) {
	payload := hookPayloadWithSession("/work/checkout-service/internal/payment/router.go",
		"22222222-2222-2222-2222-222222222222")
	block, msg := decide(payload, lockedTicket(t))
	if !block {
		t.Fatal("expected a ticket from another session to be blocked")
	}
	if !strings.Contains(msg, "SESSION_MISMATCH") || !strings.Contains(msg, "lock_in_plan") {
		t.Errorf("message should name the mismatch and guide back to lock_in_plan, got %q", msg)
	}
}

func TestPayloadWithoutSessionSkipsTheCheck(t *testing.T) {
	// Older harnesses may not send session_id: the guard stays permissive on
	// that dimension and still enforces signature, TTL, and scope.
	block, msg := decide(hookPayload("/work/checkout-service/internal/payment/router.go"), lockedTicket(t))
	if block {
		t.Fatalf("expected the check to be skipped without a payload session id, got %q", msg)
	}
}

func TestRecordSessionWritesFileAndPurgesTicket(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ticketFile), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := hookInput{HookEventName: "SessionStart", SessionID: "sess-9", CWD: dir}
	if err := recordSession(in); err != nil {
		t.Fatalf("recordSession: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, sessionFile))
	if err != nil {
		t.Fatalf("session file should exist: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "sess-9" {
		t.Errorf("session file content = %q, want sess-9", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, ticketFile)); !os.IsNotExist(err) {
		t.Error("a leftover ticket must be purged at session start")
	}
}
