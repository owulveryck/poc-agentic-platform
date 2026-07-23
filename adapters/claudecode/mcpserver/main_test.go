package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/store"
)

func TestStampSessionID_NoActiveKeepsPlanValue(t *testing.T) {
	ss := store.NewMemory()
	p := &plan.Plan{SessionID: "plan-provided"}
	if stampSessionID(p, ss) {
		t.Fatal("stampSessionID should return false when no active session")
	}
	if p.SessionID != "plan-provided" {
		t.Errorf("SessionID = %q, want plan-provided", p.SessionID)
	}
}

func TestStampSessionID_ActiveOverridesPlanValue(t *testing.T) {
	ss := store.NewMemory()
	if err := ss.PutActive("real-session-42"); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{SessionID: "agent-guessed"}
	if !stampSessionID(p, ss) {
		t.Fatal("stampSessionID should return true when an active session exists")
	}
	if p.SessionID != "real-session-42" {
		t.Errorf("SessionID = %q, want real-session-42", p.SessionID)
	}
}

// TestRegisterLocalSkills_UploadsAndDedupes fakes a gateway that captures
// register_skill calls; asserts that two skills under .claude/skills produce
// two POSTs on the first call and zero POSTs on the second (content
// unchanged), plus one extra POST after modifying one skill's rego.
func TestRegisterLocalSkills_UploadsAndDedupes(t *testing.T) {
	var count int32
	var seen []capturedReg
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/register_skill" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&count, 1)
		var body capturedReg
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		seen = append(seen, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"SKILL_REGISTERED"}`))
	}))
	t.Cleanup(fake.Close)
	t.Setenv("PPG_URL", fake.URL)

	skillsDir := filepath.Join(t.TempDir(), ".claude", "skills")
	writeSkill(t, skillsDir, "alpha", "alpha body", `package ppg.skills.alpha
import rego.v1
`)
	writeSkill(t, skillsDir, "beta", "beta body", `package ppg.skills.beta
import rego.v1
`)

	cache := &skillRegistrationCache{seen: map[string][32]byte{}}
	registerLocalSkills(context.Background(), "sess-1", []string{skillsDir}, cache)
	if got := atomic.LoadInt32(&count); got != 2 {
		t.Fatalf("first call: %d POSTs, want 2 (got names %v)", got, names(seen))
	}
	// Second call with the same content — cache hit, zero POSTs.
	registerLocalSkills(context.Background(), "sess-1", []string{skillsDir}, cache)
	if got := atomic.LoadInt32(&count); got != 2 {
		t.Fatalf("second call: %d POSTs, want still 2", got)
	}
	// Modify alpha's rego — cache miss for alpha only.
	writeSkill(t, skillsDir, "alpha", "alpha body", `package ppg.skills.alpha
import rego.v1

# modified
`)
	registerLocalSkills(context.Background(), "sess-1", []string{skillsDir}, cache)
	if got := atomic.LoadInt32(&count); got != 3 {
		t.Fatalf("third call after alpha change: %d POSTs, want 3", got)
	}
}

// TestRegisterLocalSkills_MissingDirIsSilent covers the fresh-project case
// where .claude/skills doesn't exist yet — must not log-noise or fail.
func TestRegisterLocalSkills_MissingDirIsSilent(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("gateway should not be called when the dir is missing")
		w.WriteHeader(500)
	}))
	t.Cleanup(fake.Close)
	t.Setenv("PPG_URL", fake.URL)

	cache := &skillRegistrationCache{seen: map[string][32]byte{}}
	registerLocalSkills(context.Background(), "sess-1", []string{filepath.Join(t.TempDir(), "nope")}, cache)
}

// TestUnknownSkillsIn extracts the skill names out of a lock_in_plan
// rejection body — the single JSON peek that drives the MCP retry path.
func TestUnknownSkillsIn(t *testing.T) {
	body := []byte(`{
		"status": "PLAN_REJECTED",
		"violations": [
			{"policy_id": "unknown_skill", "message": "plan declares skill_id \"foo\" but ..."},
			{"policy_id": "go_tests_present", "message": "..."}
		]
	}`)
	got := unknownSkillsIn(body)
	if len(got) != 1 || got[0] != "foo" {
		t.Fatalf("unknownSkillsIn = %v, want [foo]", got)
	}

	// PLAN_LOCKED (no violations field): nothing to retry.
	if got := unknownSkillsIn([]byte(`{"status":"PLAN_LOCKED"}`)); len(got) != 0 {
		t.Fatalf("PLAN_LOCKED body must yield no unknown skills, got %v", got)
	}
	// Malformed body: fail-safe, no retry attempted.
	if got := unknownSkillsIn([]byte(`not json`)); len(got) != 0 {
		t.Fatalf("malformed body must yield nil, got %v", got)
	}
}

// TestLockWithRegistrationRetry_ReuploadsOnUnknownSkill drives the full
// self-heal path with a fake gateway that: registers a skill, then simulates
// a restart by responding unknown_skill on the first lock, then accepts the
// re-upload, then locks the plan.
func TestLockWithRegistrationRetry_ReuploadsOnUnknownSkill(t *testing.T) {
	var (
		registerCalls int32
		lockCalls     int32
	)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/register_skill":
			atomic.AddInt32(&registerCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"SKILL_REGISTERED"}`))
		case "/lock_in_plan":
			n := atomic.AddInt32(&lockCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			if n == 1 {
				// Simulate a gateway that lost its session skills (post-restart).
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{
					"status":"PLAN_REJECTED",
					"violations":[{"policy_id":"unknown_skill","message":"plan declares skill_id \"demo\" but no published skill with that name is registered"}]
				}`))
				return
			}
			// Second attempt: the skill was re-uploaded, plan locks.
			_, _ = w.Write([]byte(`{"status":"PLAN_LOCKED","execution_ticket":"tok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.Close)
	t.Setenv("PPG_URL", fake.URL)

	skillsDir := filepath.Join(t.TempDir(), ".claude", "skills")
	writeSkill(t, skillsDir, "demo", "demo body", `package ppg.skills.demo
import rego.v1
`)

	cache := &skillRegistrationCache{seen: map[string][32]byte{}}
	body := []byte(`{"session_id":"sess-1","skill_id":"demo","intent":"x","steps":[]}`)

	raw, status, err := lockWithRegistrationRetry(context.Background(), "sess-1", []string{skillsDir}, cache, body, nil)
	if err != nil {
		t.Fatalf("lockWithRegistrationRetry: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("final status = %d, want 200 (raw: %s)", status, raw)
	}
	if !bytes.Contains(raw, []byte("PLAN_LOCKED")) {
		t.Fatalf("final body missing PLAN_LOCKED: %s", raw)
	}
	if got := atomic.LoadInt32(&lockCalls); got != 2 {
		t.Fatalf("lock_in_plan calls = %d, want 2 (initial + retry)", got)
	}
	// One initial upload (cache miss) plus one re-upload after cache.forget.
	if got := atomic.LoadInt32(&registerCalls); got != 2 {
		t.Fatalf("register_skill calls = %d, want 2 (initial + retry re-upload)", got)
	}
}

// TestLockWithRegistrationRetry_NoRetryOnHappyPath keeps the cache honest:
// when the gateway locks the plan on the first try, no retry fires and no
// extra register is issued.
func TestLockWithRegistrationRetry_NoRetryOnHappyPath(t *testing.T) {
	var (
		registerCalls int32
		lockCalls     int32
	)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/register_skill":
			atomic.AddInt32(&registerCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"SKILL_REGISTERED"}`))
		case "/lock_in_plan":
			atomic.AddInt32(&lockCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"PLAN_LOCKED","execution_ticket":"tok"}`))
		}
	}))
	t.Cleanup(fake.Close)
	t.Setenv("PPG_URL", fake.URL)

	skillsDir := filepath.Join(t.TempDir(), ".claude", "skills")
	writeSkill(t, skillsDir, "demo", "demo body", `package ppg.skills.demo
import rego.v1
`)

	cache := &skillRegistrationCache{seen: map[string][32]byte{}}
	body := []byte(`{"session_id":"sess-1"}`)
	if _, _, err := lockWithRegistrationRetry(context.Background(), "sess-1", []string{skillsDir}, cache, body, nil); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&lockCalls); got != 1 {
		t.Fatalf("happy path lock calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&registerCalls); got != 1 {
		t.Fatalf("happy path register calls = %d, want 1 (initial only)", got)
	}
}

// TestLockWithRegistrationRetry_StopsAfterOneRetry proves the retry is
// bounded: a permanently-missing skill returns the second unknown_skill
// verbatim so the model self-corrects (rather than the MCP looping).
func TestLockWithRegistrationRetry_StopsAfterOneRetry(t *testing.T) {
	var lockCalls int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/register_skill":
			// Pretend the register succeeded but the gateway keeps
			// "forgetting" — e.g. the skill isn't actually on disk. The
			// second lock must still return the unknown_skill body.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"SKILL_REGISTERED"}`))
		case "/lock_in_plan":
			atomic.AddInt32(&lockCalls, 1)
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"status":"PLAN_REJECTED","violations":[{"policy_id":"unknown_skill","message":"plan declares skill_id \"demo\" ..."}]}`))
		}
	}))
	t.Cleanup(fake.Close)
	t.Setenv("PPG_URL", fake.URL)

	skillsDir := filepath.Join(t.TempDir(), ".claude", "skills")
	writeSkill(t, skillsDir, "demo", "", `package ppg.skills.demo
import rego.v1
`)
	cache := &skillRegistrationCache{seen: map[string][32]byte{}}
	raw, status, err := lockWithRegistrationRetry(context.Background(), "sess-1", []string{skillsDir}, cache, []byte(`{"session_id":"sess-1"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&lockCalls); got != 2 {
		t.Fatalf("expected exactly 2 lock calls (initial + one retry), got %d", got)
	}
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("expected the second unknown_skill to reach the caller, got status %d body %s", status, raw)
	}
}

// TestCacheForgetSpecificAndAll covers both signatures of the cache.forget
// helper — a named skill and the wildcard-per-session sweep.
func TestCacheForgetSpecificAndAll(t *testing.T) {
	c := &skillRegistrationCache{seen: map[string][32]byte{}}
	c.seen["A|x"] = [32]byte{1}
	c.seen["A|y"] = [32]byte{2}
	c.seen["B|x"] = [32]byte{3}

	c.forget("A", "x")
	if _, ok := c.seen["A|x"]; ok {
		t.Fatal("A|x should be gone")
	}
	if _, ok := c.seen["A|y"]; !ok {
		t.Fatal("A|y should remain")
	}

	c.forget("A", "") // sweep the rest of session A
	if _, ok := c.seen["A|y"]; ok {
		t.Fatal("A|y should be gone after sweep")
	}
	if _, ok := c.seen["B|x"]; !ok {
		t.Fatal("B|x (different session) must be untouched")
	}
}

// TestSkillNameFromMD prefers the front-matter `name:` over the fallback.
func TestSkillNameFromMD(t *testing.T) {
	got := skillNameFromMD([]byte("---\nname: custom-name\n---\nbody\n"), "fallback")
	if got != "custom-name" {
		t.Errorf("got %q, want custom-name", got)
	}
	got = skillNameFromMD([]byte("no front matter\n"), "fallback")
	if got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
}

func writeSkill(t *testing.T, root, name, md, rego string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"+md+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rego != "" {
		if err := os.WriteFile(filepath.Join(dir, "SKILL.rego"), []byte(rego), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func names(regs []capturedReg) []string {
	out := make([]string, len(regs))
	for i, r := range regs {
		out[i] = r.Name
	}
	return out
}

type capturedReg struct {
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	SkillMD   string `json:"skill_md"`
	SkillRego string `json:"skill_rego"`
}

// TestDiscoverSkillDirsOrderAndPrecedence: the scan order must be user-wide,
// then .agents/skills, then project .claude/skills — last wins at the
// gateway (session tier is last-write-wins), so the project-local package
// overrides a user-wide install of the same name.
func TestDiscoverSkillDirsOrderAndPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()

	dirs := discoverSkillDirs(project)
	want := []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(project, ".agents", "skills"),
		filepath.Join(project, ".claude", "skills"),
	}
	if len(dirs) != len(want) {
		t.Fatalf("discoverSkillDirs = %v, want %v", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Fatalf("discoverSkillDirs[%d] = %q, want %q", i, dirs[i], want[i])
		}
	}

	// End-to-end: a user-wide and a project-local package under the same
	// skill name must both upload, project-local last.
	var order []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string `json:"name"`
			SkillRego string `json:"skill_rego"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		order = append(order, req.Name+":"+req.SkillRego)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"SKILL_REGISTERED"}`))
	}))
	defer srv.Close()
	t.Setenv("PPG_URL", srv.URL)

	writeSkill(t, filepath.Join(home, ".claude", "skills"), "dup", "user-wide body", "package ppg.skills.userwide\n")
	writeSkill(t, filepath.Join(project, ".claude", "skills"), "dup", "project body", "package ppg.skills.project\n")

	cache := &skillRegistrationCache{seen: map[string][32]byte{}}
	registerLocalSkills(context.Background(), "sess-p", discoverSkillDirs(project), cache)

	if len(order) != 2 {
		t.Fatalf("expected 2 uploads (user-wide then project), got %v", order)
	}
	if order[0] != "dup:package ppg.skills.userwide\n" || order[1] != "dup:package ppg.skills.project\n" {
		t.Fatalf("upload order must end with the project package (last-write-wins), got %v", order)
	}
}
