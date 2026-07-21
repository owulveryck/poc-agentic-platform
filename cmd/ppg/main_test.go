package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/catalog"
	"github.com/owulveryck/poc-agentic-platform/internal/linter"
	"github.com/owulveryck/poc-agentic-platform/internal/skill"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/dbmigrate"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/patchcode"
)

// testServer builds the real gateway (real ADR corpus + policies) and returns a
// running httptest server plus a valid execution ticket for a locked plan.
func testServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	store, err := adr.Load("../../examples/adr")
	if err != nil {
		t.Fatalf("adr.Load: %v", err)
	}
	lint, err := linter.New(store, "../../examples/adr")
	if err != nil {
		t.Fatalf("linter.New: %v", err)
	}
	skillLint, err := skill.NewLinter("../../skill-governance")
	if err != nil {
		t.Fatalf("skill.NewLinter: %v", err)
	}
	smarttools.Register(patchcode.Tool{}, "amplifier", "")
	smarttools.Register(dbmigrate.Tool{}, "amplifier", "")
	smarttools.SetArtifactEvaluator(func(path, content, skillID, sessionID string) []string {
		var msgs []string
		for _, v := range lint.EvaluateArtifact(sessionID, skillID, linter.Artifact{Path: path, Content: content}) {
			msgs = append(msgs, v.Message)
		}
		return msgs
	})

	catStore, err := catalog.Load("../../examples/services")
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	ranker, err := catalog.NewRanker("../../examples/service-policy")
	if err != nil {
		t.Fatalf("catalog.NewRanker: %v", err)
	}

	srv := httptest.NewServer(buildMux(store, lint, skillLint, catStore, ranker, time.Hour, newConflictDetector(""), filepath.Join(t.TempDir(), "escalations.jsonl")))
	t.Cleanup(srv.Close)

	planJSON := `{"session_id":"11111111-1111-1111-1111-111111111111","intent":"build a landing page","repository_context":{"name":"web","tech_stack":["Go"]},"steps":[{"id":"s1","action":"read design tokens","tool":"Read","targets":["design/tokens.css"]},{"id":"s2","action":"write styles","tool":"Write","targets":["index.css"]},{"id":"s3","action":"go test","tool":"go-test","targets":["x_test.go"]}]}`
	status, body := post(t, srv.URL+"/lock_in_plan", planJSON)
	if status != http.StatusOK {
		t.Fatalf("lock_in_plan: status %d body %s", status, body)
	}
	var locked struct {
		ExecutionTicket string `json:"execution_ticket"`
	}
	if err := json.Unmarshal([]byte(body), &locked); err != nil || locked.ExecutionTicket == "" {
		t.Fatalf("no ticket in lock response: %s", body)
	}
	return srv, locked.ExecutionTicket
}

func post(t *testing.T, url, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Errorf("POST %s: %v", url, err)
		return 0, ""
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestGatewayConcurrentRequestsAreRaceFree fires many concurrent requests at
// every endpoint sharing one linter/OPA prepared query. Run with -race to prove
// the gateway has no data race under concurrent load (the evaluation scenario).
func TestGatewayConcurrentRequestsAreRaceFree(t *testing.T) {
	srv, ticket := testServer(t)

	artifactReq := func(content string) string {
		b, _ := json.Marshal(map[string]string{"ticket": ticket, "path": "index.css", "content": content})
		return string(b)
	}
	changesetReq, _ := json.Marshal(map[string]any{
		"ticket": ticket,
		"files":  []map[string]string{{"path": "index.css", "content": ".a{color:var(--color-primary)}"}},
	})
	enrichReq := `{"intent":"add an external payment provider","repository_context":{"name":"web","tech_stack":["Go"]}}`
	skillReq := `{"name":"x","description":"short","version":"1.0.0","body":"b"}`

	const workers = 60
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			switch i % 6 {
			case 0:
				post(t, srv.URL+"/enrich", enrichReq)
			case 1:
				if s, b := post(t, srv.URL+"/verify_artifact", artifactReq(".a{color:var(--color-primary)}")); s != http.StatusOK {
					t.Errorf("verify_artifact clean: %d %s", s, b)
				}
			case 2:
				if s, _ := post(t, srv.URL+"/verify_artifact", artifactReq(".a{color:#F0F}")); s != http.StatusUnprocessableEntity {
					t.Errorf("verify_artifact raw color: want 422, got %d", s)
				}
			case 3:
				if s, b := post(t, srv.URL+"/verify_changeset", string(changesetReq)); s != http.StatusOK {
					t.Errorf("verify_changeset: %d %s", s, b)
				}
			case 4:
				resp, err := http.Get(srv.URL + "/debt_report")
				if err == nil {
					_ = resp.Body.Close()
				}
			case 5:
				post(t, srv.URL+"/validate_skill", skillReq)
			}
		}(i)
	}
	wg.Wait()
}

// TestVerifyArtifactRejectsSkillCompanionViolation locks a plan with skill_id
// "design-system" and posts a raw-hex .tsx artifact to /verify_artifact,
// proving the skill's artifact-view rule fires end-to-end through the ticket.
func TestVerifyArtifactRejectsSkillCompanionViolation(t *testing.T) {
	store, err := adr.Load("../../examples/adr")
	if err != nil {
		t.Fatalf("adr.Load: %v", err)
	}
	lint, err := linter.New(store, "../../examples/adr")
	if err != nil {
		t.Fatalf("linter.New: %v", err)
	}
	if err := lint.LoadSkillCompanions("../../demo/skills"); err != nil {
		t.Fatalf("LoadSkillCompanions: %v", err)
	}
	skillLint, err := skill.NewLinter("../../skill-governance")
	if err != nil {
		t.Fatalf("skill.NewLinter: %v", err)
	}
	catStore, err := catalog.Load("../../examples/services")
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	ranker, err := catalog.NewRanker("../../examples/service-policy")
	if err != nil {
		t.Fatalf("catalog.NewRanker: %v", err)
	}
	srv := httptest.NewServer(buildMux(store, lint, skillLint, catStore, ranker, time.Hour, newConflictDetector(""), filepath.Join(t.TempDir(), "escalations.jsonl")))
	t.Cleanup(srv.Close)

	// Plan reads design/tokens.css (ADR-090 plan rule) and writes a .tsx
	// under a skill_id the gateway knows about.
	planJSON := `{"session_id":"22222222-2222-2222-2222-222222222222","intent":"tweak the CTA","skill_id":"design-system","repository_context":{"name":"web","tech_stack":["TypeScript"]},"steps":[{"id":"s1","action":"read design tokens","tool":"Read","targets":["design/tokens.css"]},{"id":"s2","action":"write component","tool":"Write","targets":["src/Button.tsx"]}]}`
	status, body := post(t, srv.URL+"/lock_in_plan", planJSON)
	if status != http.StatusOK {
		t.Fatalf("lock_in_plan: status %d body %s", status, body)
	}
	var locked struct {
		ExecutionTicket string `json:"execution_ticket"`
	}
	if err := json.Unmarshal([]byte(body), &locked); err != nil || locked.ExecutionTicket == "" {
		t.Fatalf("no ticket in lock response: %s", body)
	}

	req, _ := json.Marshal(map[string]string{
		"ticket":  locked.ExecutionTicket,
		"path":    "src/Button.tsx",
		"content": "export const c = '#ff0000'",
	})
	status, body = post(t, srv.URL+"/verify_artifact", string(req))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 ARTIFACT_REJECTED, got %d %s", status, body)
	}
	var resp struct {
		Status     string `json:"status"`
		Violations []struct {
			PolicyID string `json:"policy_id"`
		} `json:"violations"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ARTIFACT_REJECTED" {
		t.Fatalf("expected ARTIFACT_REJECTED, got %s", resp.Status)
	}
	foundSkill := false
	for _, v := range resp.Violations {
		if v.PolicyID == "design_tokens_referenced" {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Fatalf("expected design_tokens_referenced violation, got %v", resp.Violations)
	}
}

// TestRegisterSkillThenVerifyArtifact drives the full client-upload path:
// POST /register_skill with a fresh SKILL.rego, then lock a plan under that
// skill id, then post an artifact that the freshly-registered rule rejects.
// This proves a skill installed at runtime (via APM, in the target scenario)
// is enforced end-to-end without any gateway restart.
func TestRegisterSkillThenVerifyArtifact(t *testing.T) {
	store, err := adr.Load("../../examples/adr")
	if err != nil {
		t.Fatalf("adr.Load: %v", err)
	}
	lint, err := linter.New(store, "../../examples/adr")
	if err != nil {
		t.Fatalf("linter.New: %v", err)
	}
	skillLint, err := skill.NewLinter("../../skill-governance")
	if err != nil {
		t.Fatalf("skill.NewLinter: %v", err)
	}
	catStore, err := catalog.Load("../../examples/services")
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	ranker, err := catalog.NewRanker("../../examples/service-policy")
	if err != nil {
		t.Fatalf("catalog.NewRanker: %v", err)
	}
	srv := httptest.NewServer(buildMux(store, lint, skillLint, catStore, ranker, time.Hour, newConflictDetector(""), filepath.Join(t.TempDir(), "escalations.jsonl")))
	t.Cleanup(srv.Close)

	const session = "33333333-3333-3333-3333-333333333333"
	rego := `package ppg.skills.uploaded
import rego.v1

violation contains v if {
	input.view == "artifact"
	endswith(input.artifact.path, ".tsx")
	contains(input.artifact.content, "BAD")
	v := {
		"policy_id": "uploaded_no_bad_in_tsx",
		"message":   "uploaded skill rejects BAD in .tsx",
		"nature":    "amplifier",
	}
}
`
	regReq, _ := json.Marshal(map[string]string{
		"session_id": session,
		"name":       "uploaded",
		"skill_md":   "---\nname: uploaded\ndescription: test\n---\n",
		"skill_rego": rego,
	})
	status, body := post(t, srv.URL+"/register_skill", string(regReq))
	if status != http.StatusOK {
		t.Fatalf("register_skill: status %d body %s", status, body)
	}

	// Lock a plan under this session + skill.
	planJSON := `{"session_id":"` + session + `","intent":"tweak the CTA","skill_id":"uploaded","repository_context":{"name":"web","tech_stack":["TypeScript"]},"steps":[{"id":"s1","action":"read design tokens","tool":"Read","targets":["design/tokens.css"]},{"id":"s2","action":"write component","tool":"Write","targets":["src/Button.tsx"]}]}`
	status, body = post(t, srv.URL+"/lock_in_plan", planJSON)
	if status != http.StatusOK {
		t.Fatalf("lock_in_plan: status %d body %s", status, body)
	}
	var locked struct {
		ExecutionTicket string `json:"execution_ticket"`
	}
	if err := json.Unmarshal([]byte(body), &locked); err != nil || locked.ExecutionTicket == "" {
		t.Fatalf("no ticket in lock response: %s", body)
	}

	// Post an artifact the uploaded skill rejects.
	verifyReq, _ := json.Marshal(map[string]string{
		"ticket":  locked.ExecutionTicket,
		"path":    "src/Button.tsx",
		"content": "// BAD content the uploaded skill rejects",
	})
	status, body = post(t, srv.URL+"/verify_artifact", string(verifyReq))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 ARTIFACT_REJECTED, got %d %s", status, body)
	}
	var resp struct {
		Violations []struct {
			PolicyID string `json:"policy_id"`
		} `json:"violations"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, v := range resp.Violations {
		if v.PolicyID == "uploaded_no_bad_in_tsx" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected uploaded_no_bad_in_tsx in violations, got %v", resp.Violations)
	}
}

// TestRegisterSkillRejectsMalformedRego proves the 422 SKILL_COMPILE_ERROR
// contract: bad rego surfaces synchronously to the caller so the client can
// warn the user rather than silently no-op at every subsequent verify.
func TestRegisterSkillRejectsMalformedRego(t *testing.T) {
	srv, _ := testServer(t)
	req, _ := json.Marshal(map[string]string{
		"session_id": "sess-x",
		"name":       "broken",
		"skill_rego": "package broken\nviolation contains v if { v := }\n",
	})
	status, body := post(t, srv.URL+"/register_skill", string(req))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for malformed rego, got %d %s", status, body)
	}
	if !strings.Contains(body, "SKILL_COMPILE_ERROR") {
		t.Fatalf("expected SKILL_COMPILE_ERROR in body, got %s", body)
	}
}

// TestDiscoverServiceReturnsRecommended exercises the service-catalog discovery
// endpoint against the real seed catalog + ranking policy.
func TestDiscoverServiceReturnsRecommended(t *testing.T) {
	srv, _ := testServer(t)

	// notification → notify-svc recommended, legacy-mailer surfaced as deprecated.
	status, body := post(t, srv.URL+"/discover_service", `{"capability":"notification"}`)
	if status != http.StatusOK {
		t.Fatalf("discover_service: status %d body %s", status, body)
	}
	var resp struct {
		Status      string `json:"status"`
		Recommended *struct {
			ServiceID string `json:"service_id"`
			Endpoint  string `json:"endpoint"`
			APIUsage  string `json:"api_usage"`
		} `json:"recommended"`
		Alternatives []struct {
			ServiceID string `json:"service_id"`
			Status    string `json:"status"`
			Reason    string `json:"reason"`
		} `json:"alternatives"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if resp.Status != "SERVICE_FOUND" || resp.Recommended == nil {
		t.Fatalf("expected SERVICE_FOUND with a recommendation, got %s", body)
	}
	if resp.Recommended.ServiceID != "notify-svc" {
		t.Errorf("recommended = %q, want notify-svc", resp.Recommended.ServiceID)
	}
	if resp.Recommended.Endpoint == "" || resp.Recommended.APIUsage == "" {
		t.Errorf("recommendation missing endpoint/api_usage: %+v", resp.Recommended)
	}
	var sawDeprecated bool
	for _, a := range resp.Alternatives {
		if a.ServiceID == "legacy-mailer" && a.Status == "deprecated" {
			sawDeprecated = true
		}
	}
	if !sawDeprecated {
		t.Errorf("expected legacy-mailer as a deprecated alternative, got %+v", resp.Alternatives)
	}
}

// TestDiscoverServiceDeniesForbidden confirms a forbidden provider is never
// recommended and is surfaced with a reason.
func TestDiscoverServiceDeniesForbidden(t *testing.T) {
	srv, _ := testServer(t)
	_, body := post(t, srv.URL+"/discover_service", `{"capability":"payment"}`)
	var resp struct {
		Recommended *struct {
			ServiceID string `json:"service_id"`
		} `json:"recommended"`
		Alternatives []struct {
			ServiceID string `json:"service_id"`
			Status    string `json:"status"`
		} `json:"alternatives"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if resp.Recommended == nil || resp.Recommended.ServiceID != "payments-gateway" {
		t.Fatalf("expected payments-gateway recommended, got %s", body)
	}
	for _, a := range resp.Alternatives {
		if a.ServiceID == "stripe-direct" && a.Status != "forbidden" {
			t.Errorf("stripe-direct should be forbidden, got %s", a.Status)
		}
	}
}

// TestPolicyConflictLivelockEscalation drives the livelock detector end to
// end: the same rejected plan submitted conflictThreshold times flips the
// response from 422 PLAN_REJECTED ("fix and resubmit") to 409
// POLICY_CONFLICT (a hard block naming the policies, their sources, and a
// stable conflict_id), appends an escalation record, and keeps answering
// 409 for the same violation set — including after a successful lock: an
// escalated conflict is closed by a human (`ppg escalations resolve`),
// never by agent behavior. A different violation set is counted on its own
// (422 until its own threshold).
func TestPolicyConflictLivelockEscalation(t *testing.T) {
	store, err := adr.Load("../../examples/adr")
	if err != nil {
		t.Fatalf("adr.Load: %v", err)
	}
	lint, err := linter.New(store, "../../examples/adr")
	if err != nil {
		t.Fatalf("linter.New: %v", err)
	}
	skillLint, err := skill.NewLinter("../../skill-governance")
	if err != nil {
		t.Fatalf("skill.NewLinter: %v", err)
	}
	escLog := filepath.Join(t.TempDir(), "escalations.jsonl")
	srv := httptest.NewServer(buildMux(store, lint, skillLint, nil, nil, time.Hour, newConflictDetector(""), escLog))
	t.Cleanup(srv.Close)

	// A Go plan with no test step: rejected by ADR-060 (go_tests_present),
	// deterministically, every time.
	badPlan := `{"session_id":"22222222-2222-2222-2222-222222222222","intent":"patch the payment router","repository_context":{"name":"pay","tech_stack":["Go"]},"steps":[{"id":"s1","action":"edit code","tool":"Edit","targets":["internal/payment/router.go"]}]}`

	for i := 1; i < conflictThreshold; i++ {
		status, body := post(t, srv.URL+"/lock_in_plan", badPlan)
		if status != http.StatusUnprocessableEntity || !strings.Contains(body, "PLAN_REJECTED") {
			t.Fatalf("submission %d: want 422 PLAN_REJECTED, got %d %s", i, status, body)
		}
	}
	status, body := post(t, srv.URL+"/lock_in_plan", badPlan)
	if status != http.StatusConflict {
		t.Fatalf("submission %d: want 409 POLICY_CONFLICT, got %d %s", conflictThreshold, status, body)
	}
	var conflict struct {
		Status        string            `json:"status"`
		PolicyIDs     []string          `json:"policy_ids"`
		PolicySources map[string]string `json:"policy_sources"`
	}
	if err := json.Unmarshal([]byte(body), &conflict); err != nil {
		t.Fatalf("decoding conflict response: %v (%s)", err, body)
	}
	if conflict.Status != "POLICY_CONFLICT" || len(conflict.PolicyIDs) == 0 {
		t.Fatalf("conflict payload incomplete: %s", body)
	}
	for id, src := range conflict.PolicySources {
		if src != "adr" && src != "skill" && src != "built-in" {
			t.Fatalf("policy %s has unclassified source %q", id, src)
		}
	}

	// Still blocked on the next identical submission.
	if status, body := post(t, srv.URL+"/lock_in_plan", badPlan); status != http.StatusConflict {
		t.Fatalf("post-escalation submission: want 409, got %d %s", status, body)
	}

	// The escalation was recorded.
	raw, err := os.ReadFile(escLog)
	if err != nil {
		t.Fatalf("escalation log unreadable: %v", err)
	}
	if !strings.Contains(string(raw), `"session_id":"22222222-2222-2222-2222-222222222222"`) {
		t.Fatalf("escalation log missing the session record: %s", raw)
	}

	// A different violation set is counted separately: first hit is 422.
	otherPlan := `{"session_id":"22222222-2222-2222-2222-222222222222","intent":"broad refactor","repository_context":{"name":"pay","tech_stack":["Go"]},"steps":[{"id":"s1","action":"edit everything","tool":"Edit","targets":["."]}]}`
	if status, body := post(t, srv.URL+"/lock_in_plan", otherPlan); status != http.StatusUnprocessableEntity {
		t.Fatalf("a different violation set's first rejection must be 422, got %d %s", status, body)
	}

	// A successful lock clears the session's pre-escalation counters but
	// NOT the escalated conflict: the same bad plan stays blocked until a
	// human resolves it.
	goodPlan := `{"session_id":"22222222-2222-2222-2222-222222222222","intent":"patch the payment router","repository_context":{"name":"pay","tech_stack":["Go"]},"steps":[{"id":"s1","action":"edit code","tool":"Edit","targets":["internal/payment/router.go"]},{"id":"s2","action":"go test","tool":"go-test","targets":["internal/payment/router_test.go"]}]}`
	if status, body := post(t, srv.URL+"/lock_in_plan", goodPlan); status != http.StatusOK {
		t.Fatalf("good plan must lock, got %d %s", status, body)
	}
	if status, _ := post(t, srv.URL+"/lock_in_plan", badPlan); status != http.StatusConflict {
		t.Fatalf("escalated conflict must survive a successful lock, got %d", status)
	}

	// Another session hitting the same wall is blocked immediately: an
	// escalated conflict is global, so session rotation does not reopen it.
	badPlanOtherSession := strings.Replace(badPlan, "22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333", 1)
	if status, body := post(t, srv.URL+"/lock_in_plan", badPlanOtherSession); status != http.StatusConflict {
		t.Fatalf("escalated conflict must block other sessions too, got %d %s", status, body)
	}
}

// TestPolicyConflictAlternatingSets closes the alternation escape: an agent
// alternating between two plan shapes (two different violation sets) must
// still escalate once one set has been rejected conflictThreshold times —
// rejections are counted per violation set, consecutive or not.
func TestPolicyConflictAlternatingSets(t *testing.T) {
	store, err := adr.Load("../../examples/adr")
	if err != nil {
		t.Fatalf("adr.Load: %v", err)
	}
	lint, err := linter.New(store, "../../examples/adr")
	if err != nil {
		t.Fatalf("linter.New: %v", err)
	}
	skillLint, err := skill.NewLinter("../../skill-governance")
	if err != nil {
		t.Fatalf("skill.NewLinter: %v", err)
	}
	escLog := filepath.Join(t.TempDir(), "escalations.jsonl")
	srv := httptest.NewServer(buildMux(store, lint, skillLint, nil, nil, time.Hour, newConflictDetector(""), escLog))
	t.Cleanup(srv.Close)

	// Set A: Go plan with no test step (go_tests_present).
	planA := `{"session_id":"44444444-4444-4444-4444-444444444444","intent":"patch the payment router","repository_context":{"name":"pay","tech_stack":["Go"]},"steps":[{"id":"s1","action":"edit code","tool":"Edit","targets":["internal/payment/router.go"]}]}`
	// Set B: over-broad target on top (scope_breadth_cap joins the set).
	planB := `{"session_id":"44444444-4444-4444-4444-444444444444","intent":"broad refactor","repository_context":{"name":"pay","tech_stack":["Go"]},"steps":[{"id":"s1","action":"edit everything","tool":"Edit","targets":["."]}]}`

	// A B A B — under the old consecutive-streak rule every submission
	// would reset the other's counter and nothing would ever escalate.
	for i := 0; i < conflictThreshold-1; i++ {
		if status, body := post(t, srv.URL+"/lock_in_plan", planA); status != http.StatusUnprocessableEntity {
			t.Fatalf("planA round %d: want 422, got %d %s", i+1, status, body)
		}
		if status, body := post(t, srv.URL+"/lock_in_plan", planB); status != http.StatusUnprocessableEntity {
			t.Fatalf("planB round %d: want 422, got %d %s", i+1, status, body)
		}
	}
	status, body := post(t, srv.URL+"/lock_in_plan", planA)
	if status != http.StatusConflict || !strings.Contains(body, "POLICY_CONFLICT") {
		t.Fatalf("planA's %dth rejection must escalate despite alternation, got %d %s", conflictThreshold, status, body)
	}
}

// TestConflictDetectorPersistence closes the restart escape: rejection
// counters and escalated conflicts survive a detector rebuild from the same
// state file, and `resolve` (the ppg escalations path) is the only way an
// escalated set comes back.
func TestConflictDetectorPersistence(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "conflicts.json")
	ids := []string{"go_tests_present"}

	d1 := newConflictDetector(statePath)
	for i := 1; i < conflictThreshold; i++ {
		if _, _, blocked := d1.observeRejection("sess-1", ids, "2026-07-21T00:00:00Z"); blocked {
			t.Fatalf("rejection %d must not block yet", i)
		}
	}

	// Simulated restart: a fresh detector on the same file continues the
	// count instead of starting over.
	d2 := newConflictDetector(statePath)
	count, cid, blocked := d2.observeRejection("sess-1", ids, "2026-07-21T00:01:00Z")
	if !blocked || count != conflictThreshold {
		t.Fatalf("rejection %d after restart must escalate, got count=%d blocked=%v", conflictThreshold, count, blocked)
	}

	// Another restart: the escalation itself persists, and blocks any
	// session immediately.
	d3 := newConflictDetector(statePath)
	if _, _, blocked := d3.observeRejection("sess-other", ids, "2026-07-21T00:02:00Z"); !blocked {
		t.Fatal("escalated conflict must survive restart and block other sessions")
	}

	// observeSuccess does not clear the escalation.
	d3.observeSuccess("sess-1")
	d3.observeSuccess("sess-other")
	if _, _, blocked := d3.observeRejection("sess-1", ids, "2026-07-21T00:03:00Z"); !blocked {
		t.Fatal("a successful lock must not clear an escalated conflict")
	}

	// resolve closes it (the `ppg escalations resolve` path) — and the
	// count restarts from scratch afterwards.
	if _, ok := d3.resolve(cid); !ok {
		t.Fatalf("resolve(%s) must find the escalated conflict", cid)
	}
	if count, _, blocked := d3.observeRejection("sess-1", ids, "2026-07-21T00:04:00Z"); blocked || count != 1 {
		t.Fatalf("after resolve the set must count from 1 again, got count=%d blocked=%v", count, blocked)
	}

	// syncFromDisk drops an escalation resolved on disk by another process
	// (the CLI) while keeping live counters.
	d4 := newConflictDetector(statePath)
	d4.observeRejection("sess-2", ids, "2026-07-21T00:05:00Z")
	d4.observeRejection("sess-2", ids, "2026-07-21T00:06:00Z")
	if _, _, blocked := d4.observeRejection("sess-2", ids, "2026-07-21T00:07:00Z"); !blocked {
		t.Fatal("third rejection must escalate")
	}
	cli := newConflictDetector(statePath)
	if _, ok := cli.resolve(conflictID("go_tests_present")); !ok {
		t.Fatal("CLI-side resolve must find the escalated conflict")
	}
	d4.syncFromDisk()
	if count, _, blocked := d4.observeRejection("sess-2", ids, "2026-07-21T00:08:00Z"); blocked {
		t.Fatalf("after CLI resolve + SIGHUP sync the set must not be blocked, got count=%d", count)
	}
}
