package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/store"
)

// gitRepo initializes a throwaway git repository and chdirs into it.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.invalid"},
		{"config", "user.name", "test"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitAll(t *testing.T) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "base"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func paths(files []changedFile) []string {
	var ps []string
	for _, f := range files {
		ps = append(ps, f.Path)
	}
	return ps
}

func TestChangedFilesWorkingTree(t *testing.T) {
	gitRepo(t)
	write(t, "kept.go", "package kept\n")
	commitAll(t)

	write(t, "kept.go", "package kept // modified\n")
	write(t, "new.go", "package new\n")

	files, err := changedFiles(false)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(paths(files), ",")
	if !strings.Contains(got, "kept.go") || !strings.Contains(got, "new.go") {
		t.Errorf("changed files = %v, want kept.go and new.go", got)
	}
	for _, f := range files {
		if f.Content == "" || f.Op != "write" {
			t.Errorf("file %s: content and op must be populated, got %+v", f.Path, f)
		}
	}
}

func TestChangedFilesStagedOnly(t *testing.T) {
	gitRepo(t)
	write(t, "base.go", "package base\n")
	commitAll(t)

	write(t, "staged.go", "package staged\n")
	if out, err := exec.Command("git", "add", "staged.go").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	write(t, "unstaged.go", "package unstaged\n")

	files, err := changedFiles(true)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(paths(files), ",")
	if !strings.Contains(got, "staged.go") {
		t.Errorf("staged run must include staged.go, got %v", got)
	}
	if strings.Contains(got, "unstaged.go") {
		t.Errorf("staged run must exclude unstaged.go, got %v", got)
	}
}

func TestChangedFilesSkipsDeletions(t *testing.T) {
	gitRepo(t)
	write(t, "gone.go", "package gone\n")
	commitAll(t)
	if err := os.Remove("gone.go"); err != nil {
		t.Fatal(err)
	}

	files, err := changedFiles(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("deletions must be skipped, got %v", paths(files))
	}
}

func TestHashPlanMatchesPlanHash(t *testing.T) {
	p := plan.Plan{
		SessionID: "s1",
		Intent:    "test intent",
		RepositoryContext: plan.RepoContext{
			Name: "svc", TechStack: []string{"Go"},
		},
		Steps: []plan.Step{{ID: "s1", Action: "a", Tool: "patch_code", Targets: []string{"x.go"}}},
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := hashPlan(file)
	if err != nil {
		t.Fatal(err)
	}
	want, err := p.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("hashPlan = %s, want %s", got, want)
	}
}

func TestHashPlanRejectsMalformedJSON(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(file, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := hashPlan(file); err == nil {
		t.Fatal("malformed plan JSON must be an error")
	}
}

func TestActiveTicket(t *testing.T) {
	m := store.NewMemory()
	if _, err := activeTicket(m); err == nil {
		t.Fatal("no active session must be an error (fail closed)")
	}
	if err := m.PutActive("sid-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := activeTicket(m); err == nil {
		t.Fatal("active session without a ticket must be an error (fail closed)")
	}
	if err := m.Put("sid-1", "tok-123"); err != nil {
		t.Fatal(err)
	}
	tok, err := activeTicket(m)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-123" {
		t.Errorf("ticket = %q, want tok-123", tok)
	}
}

func TestVerifyChangesetContract(t *testing.T) {
	var received struct {
		Ticket   string        `json:"ticket"`
		Files    []changedFile `json:"files"`
		PlanHash string        `json:"plan_hash"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/verify_changeset" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"status":"CHANGESET_REJECTED","violations":[{"message":"ADR-090 violated"}],"guidance":"fix it"}`))
	}))
	defer srv.Close()

	status, msgs, err := verifyChangeset(srv.URL, "tok", []changedFile{{Path: "a.go", Content: "x", Op: "write"}}, "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if status != "CHANGESET_REJECTED" {
		t.Errorf("status = %s", status)
	}
	if len(msgs) != 2 || msgs[0] != "ADR-090 violated" || msgs[1] != "fix it" {
		t.Errorf("messages = %v", msgs)
	}
	if received.Ticket != "tok" || received.PlanHash != "hash-1" || len(received.Files) != 1 {
		t.Errorf("request payload = %+v", received)
	}
}

func TestVerifyChangesetGatewayDownFailsClosed(t *testing.T) {
	if _, _, err := verifyChangeset("http://127.0.0.1:1", "tok", []changedFile{{Path: "a.go"}}, ""); err == nil {
		t.Fatal("unreachable gateway must be an error (fail closed)")
	}
}

func TestGatewayURLEnvOverride(t *testing.T) {
	t.Setenv("PPG_URL", "http://example.invalid:9999")
	if got := gatewayURL(); got != "http://example.invalid:9999" {
		t.Errorf("gatewayURL = %s", got)
	}
}
