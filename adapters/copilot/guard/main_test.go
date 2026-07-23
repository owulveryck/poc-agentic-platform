package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// editPayload mimics the real Copilot desktop-app payload for the `Edit`
// tool: file path lives under tool_input.path.
func editPayload(path string) []byte {
	return []byte(`{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/work/checkout-service","tool_input":{"path":"` + path + `"}}`)
}

func editPayloadWithSession(path, sessionID string) []byte {
	return []byte(`{"hook_event_name":"PreToolUse","tool_name":"Edit","session_id":"` + sessionID + `","cwd":"/work/checkout-service","tool_input":{"path":"` + path + `"}}`)
}

// vscodeEditFilesPayload mimics the VS Code Copilot Chat variant: tool
// name is `editFiles` and the field is `file_path`.
func vscodeEditFilesPayload(filePath string) []byte {
	return []byte(`{"hook_event_name":"PreToolUse","tool_name":"editFiles","cwd":"/work/checkout-service","tool_input":{"file_path":"` + filePath + `"}}`)
}

func TestInScopeEditPasses(t *testing.T) {
	block, _, msg := decide(editPayload("/work/checkout-service/internal/payment/router.go"), lockedTicket(t), nil)
	if block {
		t.Fatalf("expected the edit to pass, got block with %q", msg)
	}
}

func TestOutOfScopeEditIsBlockedWithSemanticMessage(t *testing.T) {
	block, _, msg := decide(editPayload("/work/checkout-service/internal/auth/login.go"), lockedTicket(t), nil)
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
	block, _, msg := decide(editPayload("/work/checkout-service/internal/payment/router.go"), "", nil)
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
	if block, _, msg := decide(editPayload(planFile), lockedTicket(t), nil); block {
		t.Fatalf("plan file write should be exempt with a ticket, got block with %q", msg)
	}
	// Exempt even with no ticket: the exemption must precede the
	// missing-ticket branch so plan-mode writes are never blocked.
	if block, _, msg := decide(editPayload(planFile), "", nil); block {
		t.Fatalf("plan file write should be exempt without a ticket, got block with %q", msg)
	}
}

func TestReadToolIsIgnored(t *testing.T) {
	// Copilot's Read/Glob/Bash tools are outside this guard's scope.
	block, _, _ := decide([]byte(`{"hook_event_name":"PreToolUse","tool_name":"Read","cwd":"/work","tool_input":{"path":"/work/x.go"}}`), "", nil)
	if block {
		t.Fatal("a Read tool call must not be blocked by the edit guard")
	}
}

func TestEditWithoutPathFailsClosed(t *testing.T) {
	// A file-mutating tool with no discernible target path is suspicious: the
	// hardened guard denies it (fail-closed) rather than letting it through.
	block, _, msg := decide([]byte(`{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/work","tool_input":{}}`), "", nil)
	if !block {
		t.Fatal("an Edit call without a path must fail closed")
	}
	if !strings.Contains(msg, "PPG_GUARD_ERROR") {
		t.Errorf("expected a fail-closed message, got %q", msg)
	}
}

func TestContentViolationBlocks(t *testing.T) {
	payload := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Write","cwd":"/work/checkout-service","tool_input":{"path":"/work/checkout-service/internal/payment/router.go","new_str":"raw"}}`)
	verify := func(ticket, path, content string) ([]string, error) { return []string{"raw color forbidden"}, nil }
	block, _, msg := decide(payload, lockedTicket(t), verify)
	if !block {
		t.Fatal("expected a content-policy violation to block")
	}
	if !strings.Contains(msg, "ARCHITECTURAL_INVARIANT_VIOLATION") {
		t.Errorf("expected the invariant message, got %q", msg)
	}
}

func TestContentVerifierErrorFailsClosed(t *testing.T) {
	payload := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Write","cwd":"/work/checkout-service","tool_input":{"path":"/work/checkout-service/internal/payment/router.go","new_str":"x"}}`)
	verify := func(ticket, path, content string) ([]string, error) { return nil, errors.New("gateway unreachable") }
	block, _, msg := decide(payload, lockedTicket(t), verify)
	if !block {
		t.Fatal("expected a verifier error to fail closed")
	}
	if !strings.Contains(msg, "PPG_GUARD_ERROR") {
		t.Errorf("expected a fail-closed message, got %q", msg)
	}
}

func TestVSCodeEditFilesPayloadShapeIsHandled(t *testing.T) {
	block, _, msg := decide(vscodeEditFilesPayload("/work/checkout-service/internal/payment/router.go"), lockedTicket(t), nil)
	if block {
		t.Fatalf("expected the editFiles/file_path shape to pass in scope, got block with %q", msg)
	}
}

func TestMatchingSessionPasses(t *testing.T) {
	payload := editPayloadWithSession("/work/checkout-service/internal/payment/router.go",
		"11111111-1111-1111-1111-111111111111")
	block, _, msg := decide(payload, lockedTicket(t), nil)
	if block {
		t.Fatalf("expected the matching session to pass, got block with %q", msg)
	}
}

func TestSessionMismatchIsBlocked(t *testing.T) {
	payload := editPayloadWithSession("/work/checkout-service/internal/payment/router.go",
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
	block, _, msg := decide(editPayload("/work/checkout-service/internal/payment/router.go"), lockedTicket(t), nil)
	if block {
		t.Fatalf("expected the check to be skipped without a payload session id, got %q", msg)
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

// TestDenyOutputShape verifies the on-wire JSON matches the VS Code Copilot
// hook contract: hookSpecificOutput.permissionDecision = "deny".
func TestDenyOutputShape(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	emitDeny("nope")
	_ = w.Close()
	os.Stdout = orig
	var out struct {
		HookSpecificOutput struct {
			HookEventName            string `json:"hookEventName"`
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("permissionDecision = %q, want %q",
			out.HookSpecificOutput.PermissionDecision, "deny")
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("hookEventName = %q, want %q",
			out.HookSpecificOutput.HookEventName, "PreToolUse")
	}
	if out.HookSpecificOutput.PermissionDecisionReason != "nope" {
		t.Errorf("permissionDecisionReason = %q, want %q",
			out.HookSpecificOutput.PermissionDecisionReason, "nope")
	}
}
