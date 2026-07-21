package linter

import (
	"strings"
	"sync"
	"testing"
)

// A minimal SKILL.rego source rejecting any .tsx artifact containing "BAD".
const artifactRejectBadTSX = `package ppg.skills.demo

import rego.v1

violation contains v if {
	input.view == "artifact"
	endswith(input.artifact.path, ".tsx")
	contains(input.artifact.content, "BAD")
	v := {
		"policy_id": "demo_no_bad_in_tsx",
		"message":   "demo skill rejects .tsx content containing BAD",
		"nature":    "amplifier",
	}
}
`

const artifactRejectFooTSX = `package ppg.skills.demo

import rego.v1

violation contains v if {
	input.view == "artifact"
	endswith(input.artifact.path, ".tsx")
	contains(input.artifact.content, "FOO")
	v := {
		"policy_id": "demo_no_foo_in_tsx",
		"message":   "demo skill (session B) rejects .tsx content containing FOO",
		"nature":    "amplifier",
	}
}
`

func TestRegisterSessionSkillFiresAtArtifactView(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const session = "session-A"
	if err := lint.RegisterSessionSkill(session, "demo", artifactRejectBadTSX); err != nil {
		t.Fatalf("RegisterSessionSkill: %v", err)
	}
	if got := lint.SessionSkillCount(session); got != 1 {
		t.Fatalf("SessionSkillCount(%q) = %d, want 1", session, got)
	}

	vs := lint.EvaluateArtifact(session, "demo", Artifact{Path: "src/x.tsx", Content: "// BAD"})
	if !hasPolicy(vs, "demo_no_bad_in_tsx") {
		t.Fatalf("expected demo_no_bad_in_tsx violation, got %v", vs)
	}

	clean := lint.EvaluateArtifact(session, "demo", Artifact{Path: "src/x.tsx", Content: "// good"})
	for _, v := range clean {
		if v.PolicyID == "demo_no_bad_in_tsx" {
			t.Fatalf("clean content should not fire demo_no_bad_in_tsx: %v", clean)
		}
	}
}

// TestSessionSkillsAreIsolated proves that two sessions registering the same
// skill name with different rego bodies do not leak into each other.
func TestSessionSkillsAreIsolated(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := lint.RegisterSessionSkill("A", "demo", artifactRejectBadTSX); err != nil {
		t.Fatal(err)
	}
	if err := lint.RegisterSessionSkill("B", "demo", artifactRejectFooTSX); err != nil {
		t.Fatal(err)
	}

	// A's rule fires on "BAD", not on "FOO".
	vsA := lint.EvaluateArtifact("A", "demo", Artifact{Path: "x.tsx", Content: "FOO not bad"})
	if hasPolicy(vsA, "demo_no_foo_in_tsx") {
		t.Fatalf("session A should not see session B's rule: %v", vsA)
	}
	if hasPolicy(vsA, "demo_no_bad_in_tsx") {
		t.Fatalf("session A's rule should not fire on FOO: %v", vsA)
	}
	// B's rule fires on "FOO", not on "BAD".
	vsB := lint.EvaluateArtifact("B", "demo", Artifact{Path: "x.tsx", Content: "BAD not foo"})
	if hasPolicy(vsB, "demo_no_bad_in_tsx") {
		t.Fatalf("session B should not see session A's rule: %v", vsB)
	}
	if hasPolicy(vsB, "demo_no_foo_in_tsx") {
		t.Fatalf("session B's rule should not fire on BAD: %v", vsB)
	}
}

// TestOperatorTierWinsOnNameCollision registers a session-scoped skill under
// a name the operator has already loaded via -skills. The operator's rule
// must fire, not the session upload — the precedence rule that keeps
// project-local rego from silently downgrading an org-wide policy.
func TestOperatorTierWinsOnNameCollision(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := lint.LoadSkillCompanions("../../demo/skills"); err != nil {
		t.Fatalf("LoadSkillCompanions: %v", err)
	}
	// Try to override the design-system skill with a permissive stub.
	permissive := `package ppg.skills.permissive
import rego.v1
`
	if err := lint.RegisterSessionSkill("A", "design-system", permissive); err != nil {
		t.Fatalf("RegisterSessionSkill: %v", err)
	}
	// The design-system skill's artifact rule still rejects raw hex in .tsx.
	vs := lint.EvaluateArtifact("A", "design-system", Artifact{
		Path:    "src/Button.tsx",
		Content: "export const c = '#ff0000'",
	})
	if !hasPolicy(vs, "design_tokens_referenced") {
		t.Fatalf("operator tier must win: expected design_tokens_referenced, got %v", vs)
	}
}

// TestUnknownSessionSkillFailsClosed makes sure a session id / skill id
// combination the linter has never seen still produces unknown_skill, so a
// forgotten registration cannot silently succeed.
func TestUnknownSessionSkillFailsClosed(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	vs := lint.EvaluateArtifact("no-such-session", "no-such-skill", Artifact{
		Path:    "src/Button.tsx",
		Content: "clean",
	})
	if !hasPolicy(vs, "unknown_skill") {
		t.Fatalf("expected unknown_skill fail-closed, got %v", vs)
	}
}

// TestUnregisterSessionDropsSkills proves the eviction hook works.
func TestUnregisterSessionDropsSkills(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := lint.RegisterSessionSkill("A", "demo", artifactRejectBadTSX); err != nil {
		t.Fatal(err)
	}
	lint.UnregisterSession("A")
	if got := lint.SessionSkillCount("A"); got != 0 {
		t.Fatalf("SessionSkillCount after Unregister = %d, want 0", got)
	}
	vs := lint.EvaluateArtifact("A", "demo", Artifact{Path: "x.tsx", Content: "BAD"})
	if !hasPolicy(vs, "unknown_skill") {
		t.Fatalf("evicted skill should return unknown_skill, got %v", vs)
	}
}

// TestRegisterSessionSkillReportsCompileError surfaces bad rego to the caller
// (which becomes 422 SKILL_COMPILE_ERROR at the HTTP layer) instead of
// storing a broken evaluator that silently no-ops later.
func TestRegisterSessionSkillReportsCompileError(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Malformed rego: missing right-hand side of the assignment.
	broken := `package ppg.skills.broken
import rego.v1

violation contains v if {
	v := }
`
	err = lint.RegisterSessionSkill("A", "broken", broken)
	if err == nil {
		t.Fatal("expected a compile error, got nil")
	}
	if !strings.Contains(err.Error(), "OPA") && !strings.Contains(err.Error(), "rego") && !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "policy") {
		t.Fatalf("compile error should mention rego/opa/parse/policy, got %v", err)
	}
	// Nothing was stored.
	if got := lint.SessionSkillCount("A"); got != 0 {
		t.Fatalf("compile error must not leave a partial registration; got %d skills", got)
	}
}

// TestTier0SessionSkillIsNoOp registers a skill with no rego (an empty
// source). Subsequent evaluations must not fire unknown_skill (the skill IS
// registered) but must not produce any violations either.
func TestTier0SessionSkillIsNoOp(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := lint.RegisterSessionSkill("A", "tier0", ""); err != nil {
		t.Fatalf("empty rego should register as tier-0, got %v", err)
	}
	vs := lint.EvaluateArtifact("A", "tier0", Artifact{Path: "x.tsx", Content: "anything"})
	for _, v := range vs {
		if v.PolicyID == "unknown_skill" {
			t.Fatalf("tier-0 registered skill should not be unknown: %v", vs)
		}
	}
}

// TestConcurrentRegisterAndEvaluate hammers register+read to prove the
// mutex is enough under -race.
func TestConcurrentRegisterAndEvaluate(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const N = 40
	var wg sync.WaitGroup
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			sid := "sess"
			_ = lint.RegisterSessionSkill(sid, "demo", artifactRejectBadTSX)
		}(i)
		go func(i int) {
			defer wg.Done()
			_ = lint.EvaluateArtifact("sess", "demo", Artifact{Path: "x.tsx", Content: "BAD"})
		}(i)
	}
	wg.Wait()
}

// TestUnionSessionSkillAppliesWithoutDeclaredSkillID: a plan/edit that omits
// skill_id must no longer bypass an installed skill's content rules — the
// union semantics of the governed-machine intent.
func TestUnionSessionSkillAppliesWithoutDeclaredSkillID(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := lint.RegisterSessionSkill("A", "demo", artifactRejectBadTSX); err != nil {
		t.Fatal(err)
	}
	vs := lint.EvaluateArtifact("A", "", Artifact{Path: "x.tsx", Content: "BAD"})
	if !hasPolicy(vs, "demo_no_bad_in_tsx") {
		t.Fatalf("union semantics: session skill must fire with no declared skill_id, got %v", vs)
	}
	// Another session must not inherit A's upload.
	vsB := lint.EvaluateArtifact("B", "", Artifact{Path: "x.tsx", Content: "BAD"})
	if hasPolicy(vsB, "demo_no_bad_in_tsx") {
		t.Fatalf("union must stay session-scoped, got %v", vsB)
	}
}

// TestUnionOperatorSkillProtectsTokensFileWithoutSkillID: with the demo
// skills loaded operator-side, an in-session write to design/tokens.css is
// refused at the artifact altitude even when no skill_id was declared —
// the content-altitude closure of the tokens-file bypass.
func TestUnionOperatorSkillProtectsTokensFileWithoutSkillID(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := lint.LoadSkillCompanions("../../demo/skills"); err != nil {
		t.Fatalf("LoadSkillCompanions: %v", err)
	}
	vs := lint.EvaluateArtifact("any-session", "", Artifact{
		Path:    "design/tokens.css",
		Content: ":root { --color-primary: #FF69B4; }",
	})
	if !hasPolicy(vs, "design_tokens_immutable") {
		t.Fatalf("union semantics: tokens-file write must be refused without a declared skill, got %v", vs)
	}

	// Changeset altitude, same protection.
	cs := lint.EvaluateChangeset("any-session", "", Changeset{Files: []Artifact{
		{Path: "design/tokens.css", Content: ":root { --color-primary: #FF69B4; }"},
	}})
	if !hasPolicy(cs, "design_tokens_immutable") {
		t.Fatalf("union semantics: tokens-file change must be refused at changeset view, got %v", cs)
	}
}

// TestUnionSkipsDeclaredSkillDoubleEvaluation: the declared skill is
// evaluated once (fail-closed path), not twice via the union walk.
func TestUnionSkipsDeclaredSkillDoubleEvaluation(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := lint.RegisterSessionSkill("A", "demo", artifactRejectBadTSX); err != nil {
		t.Fatal(err)
	}
	vs := lint.EvaluateArtifact("A", "demo", Artifact{Path: "x.tsx", Content: "BAD"})
	n := 0
	for _, v := range vs {
		if v.PolicyID == "demo_no_bad_in_tsx" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("declared skill must be evaluated exactly once, fired %d times: %v", n, vs)
	}
}
