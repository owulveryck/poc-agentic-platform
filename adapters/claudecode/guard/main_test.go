package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/journal"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/store"
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
	block, _, msg := decide(hookPayload("/work/checkout-service/internal/payment/router.go"), lockedTicket(t), nil)
	if block {
		t.Fatalf("expected the edit to pass, got block with %q", msg)
	}
}

func TestOutOfScopeEditIsBlockedWithSemanticMessage(t *testing.T) {
	block, _, msg := decide(hookPayload("/work/checkout-service/internal/auth/login.go"), lockedTicket(t), nil)
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
	block, _, msg := decide(hookPayload("/work/checkout-service/internal/payment/router.go"), "", nil)
	if !block {
		t.Fatal("expected a missing ticket to block")
	}
	if !strings.Contains(msg, "No capability ticket") || !strings.Contains(msg, "lock_in_plan") {
		t.Errorf("message should guide the agent to lock a plan, got %q", msg)
	}
}

func TestPlanFileIsExempt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	planFile := filepath.Join(home, ".claude", "plans", "some-plan.md")

	// Exempt with a valid ticket: a plan file is never a product edit.
	if block, _, msg := decide(hookPayload(planFile), lockedTicket(t), nil); block {
		t.Fatalf("plan file write should be exempt with a ticket, got block with %q", msg)
	}
	// Exempt even with no ticket: plan mode writes its plan before any plan
	// is locked, so the exemption must precede the missing-ticket branch.
	if block, _, msg := decide(hookPayload(planFile), "", nil); block {
		t.Fatalf("plan file write should be exempt without a ticket, got block with %q", msg)
	}
}

func TestNonFileToolIsIgnored(t *testing.T) {
	block, _, _ := decide([]byte(`{"tool_name":"Bash","cwd":"/work","tool_input":{"command":"ls"}}`), "", nil)
	if block {
		t.Fatal("a tool call without file_path must not be blocked by the guard")
	}
}

func TestMatchingSessionPasses(t *testing.T) {
	payload := hookPayloadWithSession("/work/checkout-service/internal/payment/router.go",
		"11111111-1111-1111-1111-111111111111")
	block, _, msg := decide(payload, lockedTicket(t), nil)
	if block {
		t.Fatalf("expected the matching session to pass, got block with %q", msg)
	}
}

func TestSessionMismatchIsBlocked(t *testing.T) {
	payload := hookPayloadWithSession("/work/checkout-service/internal/payment/router.go",
		"22222222-2222-2222-2222-222222222222")
	block, _, msg := decide(payload, lockedTicket(t), nil)
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
	block, _, msg := decide(hookPayload("/work/checkout-service/internal/payment/router.go"), lockedTicket(t), nil)
	if block {
		t.Fatalf("expected the check to be skipped without a payload session id, got %q", msg)
	}
}

func notebookPayload(notebookPath string) []byte {
	return []byte(`{"tool_name":"NotebookEdit","cwd":"/work/checkout-service","tool_input":{"notebook_path":"` + notebookPath + `"}}`)
}

func TestNotebookEditIsGuardedViaNotebookPath(t *testing.T) {
	// Regression: NotebookEdit carries its path in notebook_path, not file_path.
	// An out-of-scope notebook write must be blocked, not silently allowed.
	block, _, msg := decide(notebookPayload("/work/checkout-service/internal/auth/secrets.ipynb"), lockedTicket(t), nil)
	if !block {
		t.Fatal("expected an out-of-scope NotebookEdit to be blocked")
	}
	if !strings.Contains(msg, "OUT_OF_PLAN_SCOPE") {
		t.Errorf("expected OUT_OF_PLAN_SCOPE, got %q", msg)
	}
}

func TestContentViolationBlocks(t *testing.T) {
	payload := []byte(`{"tool_name":"Write","cwd":"/work/checkout-service","tool_input":{"file_path":"/work/checkout-service/internal/payment/router.go","content":"raw"}}`)
	verify := func(ticket, path, content string) ([]string, error) {
		return []string{"raw color forbidden"}, nil
	}
	block, _, msg := decide(payload, lockedTicket(t), verify)
	if !block {
		t.Fatal("expected a content-policy violation to block")
	}
	if !strings.Contains(msg, "ARCHITECTURAL_INVARIANT_VIOLATION") || !strings.Contains(msg, "raw color forbidden") {
		t.Errorf("expected the invariant message, got %q", msg)
	}
}

func TestContentVerifierErrorFailsClosed(t *testing.T) {
	payload := []byte(`{"tool_name":"Write","cwd":"/work/checkout-service","tool_input":{"file_path":"/work/checkout-service/internal/payment/router.go","content":"x"}}`)
	verify := func(ticket, path, content string) ([]string, error) {
		return nil, errors.New("gateway unreachable")
	}
	block, _, msg := decide(payload, lockedTicket(t), verify)
	if !block {
		t.Fatal("expected a verifier error to fail closed (block)")
	}
	if !strings.Contains(msg, "PPG_GUARD_ERROR") {
		t.Errorf("expected a fail-closed message, got %q", msg)
	}
}

func TestCleanContentPasses(t *testing.T) {
	payload := []byte(`{"tool_name":"Write","cwd":"/work/checkout-service","tool_input":{"file_path":"/work/checkout-service/internal/payment/router.go","content":"ok"}}`)
	verify := func(ticket, path, content string) ([]string, error) { return nil, nil }
	if block, _, msg := decide(payload, lockedTicket(t), verify); block {
		t.Fatalf("expected clean in-scope content to pass, got %q", msg)
	}
}

func TestRecordSessionRecordsAndPurgesStaleTickets(t *testing.T) {
	st := store.NewMemory()
	if err := st.Put("previous-session", "stale-jwt"); err != nil {
		t.Fatal(err)
	}
	in := hookInput{HookEventName: "SessionStart", SessionID: "sess-9"}
	if err := recordSession(in, st, st); err != nil {
		t.Fatalf("recordSession: %v", err)
	}
	got, err := st.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got != "sess-9" {
		t.Errorf("active session = %q, want sess-9", got)
	}
	if _, err := st.Get("previous-session"); !errors.Is(err, store.ErrNotFound) {
		t.Error("a leftover ticket from a previous session must be purged at session start")
	}
}

func TestRecordSessionEmptyIDIsNoop(t *testing.T) {
	st := store.NewMemory()
	if err := st.Put("keep", "jwt"); err != nil {
		t.Fatal(err)
	}
	in := hookInput{HookEventName: "SessionStart"}
	if err := recordSession(in, st, st); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetActive(); !errors.Is(err, store.ErrNotFound) {
		t.Error("empty session id should not set an active session")
	}
	if _, err := st.Get("keep"); err != nil {
		t.Error("empty session id should not purge tickets")
	}
}

func TestReadTicketPreferPayloadSessionID(t *testing.T) {
	st := store.NewMemory()
	_ = st.PutActive("stale-active")
	_ = st.Put("payload-sess", "the-jwt")
	got := readTicket(hookInput{SessionID: "payload-sess"}, st, st)
	if got != "the-jwt" {
		t.Errorf("readTicket = %q, want the-jwt", got)
	}
}

func TestReadTicketFallbackToActiveSession(t *testing.T) {
	st := store.NewMemory()
	_ = st.PutActive("current")
	_ = st.Put("current", "the-jwt")
	got := readTicket(hookInput{}, st, st)
	if got != "the-jwt" {
		t.Errorf("readTicket without payload sid = %q, want the-jwt", got)
	}
}

func TestReadTicketReturnsEmptyWhenNoTicket(t *testing.T) {
	st := store.NewMemory()
	if got := readTicket(hookInput{SessionID: "nobody"}, st, st); got != "" {
		t.Errorf("readTicket without stored ticket = %q, want empty", got)
	}
}

// TestDecideVerdictCodes asserts the machine-readable verdict code of every
// decide branch — the reason_code attribute the journal's ppg.guard.block
// events (and ppg report) are built on.
func TestDecideVerdictCodes(t *testing.T) {
	inScope := "/work/checkout-service/internal/payment/router.go"
	failingVerify := func(_, _, _ string) ([]string, error) { return nil, errors.New("server down") }
	violatingVerify := func(_, _, _ string) ([]string, error) { return []string{"raw hex color"}, nil }

	cases := []struct {
		name      string
		payload   []byte
		ticket    string
		verify    artifactVerifier
		wantBlock bool
		wantCode  string
	}{
		{"non-write tool", []byte(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`), "", nil, false, verdictNone},
		{"write passes all gates", hookPayload(inScope), lockedTicket(t), nil, false, verdictAllow},
		{"no ticket", hookPayload(inScope), "", nil, true, journal.ReasonNoTicket},
		{"out of plan scope", hookPayload("/work/checkout-service/internal/auth/login.go"), lockedTicket(t), nil, true, journal.ReasonOutOfPlanScope},
		{"ticket rejected", hookPayload(inScope), "not-a-jwt", nil, true, journal.ReasonTicketRejected},
		{"session mismatch", hookPayloadWithSession(inScope, "22222222-2222-2222-2222-222222222222"), lockedTicket(t), nil, true, journal.ReasonSessionMismatch},
		{"invariant violation", hookPayload(inScope), lockedTicket(t), violatingVerify, true, journal.ReasonInvariantViolation},
		{"verify unreachable", hookPayload(inScope), lockedTicket(t), failingVerify, true, journal.ReasonGuardError},
		{"missing target path", []byte(`{"tool_name":"Edit","tool_input":{}}`), lockedTicket(t), nil, true, journal.ReasonGuardError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block, code, _ := decide(tc.payload, tc.ticket, tc.verify)
			if block != tc.wantBlock || code != tc.wantCode {
				t.Fatalf("decide = (block=%v, code=%q), want (block=%v, code=%q)", block, code, tc.wantBlock, tc.wantCode)
			}
		})
	}
}
