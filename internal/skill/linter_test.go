package skill

import (
	"testing"
)

func validSkill() *Skill {
	return &Skill{
		Name:        "patch-payment",
		Description: "Applies targeted changes to the payment service, following platform ADRs for proxy and migration ordering.",
		Version:     "1.0.0",
		Body:        "Analyse the intent and produce a plan. Use Read to inspect files.",
	}
}

func TestValidSkillPasses(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	violations := lint.Validate(validSkill())
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %v", violations)
	}
}

func TestMissingNameIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Name = ""
	assertViolation(t, lint, s, "name")
}

func TestInvalidNameFormatIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Name = "PatchPayment" // uppercase not allowed
	assertViolation(t, lint, s, "name")
}

func TestShortDescriptionIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Description = "Too short."
	assertViolation(t, lint, s, "description")
}

func TestMissingVersionIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Version = ""
	assertViolation(t, lint, s, "version")
}

func TestArgumentsWithoutHintIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Apply $ARGUMENTS to the payment router."
	assertViolation(t, lint, s, "argument_hint")
}

func TestFileModificationWithoutRegoIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Use Edit to patch the router file."
	// no RegoPolicy → tier-1 skill without companion
	assertViolation(t, lint, s, "rego_policy")
}

func TestFileModificationWithRegoPassses(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Use Edit to patch the router file."
	s.RegoPolicy = `package ppg.skills.patch_payment
import rego.v1
# companion policy placeholder`
	violations := lint.Validate(s)
	for _, v := range violations {
		if v.Field == "rego_policy" {
			t.Fatalf("unexpected rego_policy violation when companion is present: %v", v)
		}
	}
}

func TestTierClassification(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	cases := []struct {
		body string
		want int
	}{
		{"Use Read and Glob to inspect the codebase.", 0},
		{"Use Edit to patch the file.", 1},
		{"Use Write to create a new file.", 1},
		{"Use Bash to run tests.", 2},
	}
	for _, tc := range cases {
		s := validSkill()
		s.Body = tc.body
		if got := lint.Tier(s); got != tc.want {
			t.Errorf("Tier(%q) = %d, want %d", tc.body, got, tc.want)
		}
	}
}

func TestUndecodableViolationFailsClosed(t *testing.T) {
	lint, err := NewLinter("testdata/badshape")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	violations := lint.Validate(validSkill())
	if len(violations) == 0 {
		t.Fatal("expected a linter violation when the OPA result cannot be decoded (fail closed), got none")
	}
	if violations[0].Field != "linter" {
		t.Fatalf("expected field \"linter\", got %v", violations)
	}
}

func assertViolation(t *testing.T, lint *Linter, s *Skill, field string) {
	t.Helper()
	violations := lint.Validate(s)
	for _, v := range violations {
		if v.Field == field {
			return
		}
	}
	t.Fatalf("expected violation for field %q, got %v", field, violations)
}
