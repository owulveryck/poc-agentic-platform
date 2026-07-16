package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
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
	store, err := adr.Load("../../adr")
	if err != nil {
		t.Fatalf("adr.Load: %v", err)
	}
	lint, err := linter.New(store, "../../adr")
	if err != nil {
		t.Fatalf("linter.New: %v", err)
	}
	skillLint, err := skill.NewLinter("../../skill-governance")
	if err != nil {
		t.Fatalf("skill.NewLinter: %v", err)
	}
	smarttools.Register(patchcode.Tool{}, "amplifier", "")
	smarttools.Register(dbmigrate.Tool{}, "amplifier", "")
	smarttools.SetArtifactEvaluator(func(path, content string) []string {
		var msgs []string
		for _, v := range lint.EvaluateArtifact(linter.Artifact{Path: path, Content: content}) {
			msgs = append(msgs, v.Message)
		}
		return msgs
	})

	srv := httptest.NewServer(buildMux(store, lint, skillLint, time.Hour))
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
