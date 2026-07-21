package skill

import (
	"strings"
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

func TestDescriptionWithoutLeadingVerbIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	// A noun phrase, no leading verb. (Note the assumed naivety of the
	// pattern: "This skill..." would pass, since "This" ends in s.)
	s.Description = "Payment provider integration workflow for the checkout service, following platform ADRs."
	assertViolation(t, lint, s, "description")
}

func TestBodyOverFiveHundredLinesIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = strings.Repeat("Inspect the code with Read.\n", 501)
	assertViolation(t, lint, s, "body")
}

func TestHardcodedSecretIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Use Read to inspect files. Authenticate with api_key = \"sk-live-123456\"."
	assertViolation(t, lint, s, "body")
}

func TestShellWithoutRegoIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Use Bash to run the integration test suite."
	// tier 2 without a companion policy: the gate must refuse
	assertViolation(t, lint, s, "rego_policy")
}

func TestShellWithRegoPasses(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Use Bash to run the integration test suite."
	s.RegoPolicy = "package ppg.skills.demo\nimport rego.v1\n"
	for _, v := range lint.Validate(s) {
		if v.Field == "rego_policy" {
			t.Fatalf("unexpected rego_policy violation when companion is present: %v", v)
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

// TestBrokenCompanionRegoIsRejectedAtPublish: Gate 1 compiles the bundled
// SKILL.rego — a syntactically broken companion is refused at publish time,
// not discovered at gateway startup.
func TestBrokenCompanionRegoIsRejectedAtPublish(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Use Edit to patch the router file."
	s.RegoPolicy = "package ppg.skills.broken\nimport rego.v1\n\nviolation contains v if {\n\tv := }\n"
	assertViolation(t, lint, s, "rego_policy")
}

// TestCompanionWithoutPackageIsRejected: a companion without a package
// declaration cannot be queried by the gateway; refuse it at publish.
func TestCompanionWithoutPackageIsRejected(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Use Edit to patch the router file."
	s.RegoPolicy = "# no package here\n"
	assertViolation(t, lint, s, "rego_policy")
}

// TestNondeterministicCompanionIsRejectedAtPublish: the publish gate uses
// the same deterministic capability set as the gateway, so a companion
// calling http.send is refused at Gate 1.
func TestNondeterministicCompanionIsRejectedAtPublish(t *testing.T) {
	lint, err := NewLinter("testdata")
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	s := validSkill()
	s.Body = "Use Edit to patch the router file."
	s.RegoPolicy = "package ppg.skills.evil\nimport rego.v1\n\nviolation contains v if {\n\tresp := http.send({\"method\": \"GET\", \"url\": \"http://example.com\"})\n\tresp.status_code == 200\n\tv := {\"field\": \"x\", \"message\": \"x\", \"nature\": \"amplifier\"}\n}\n"
	assertViolation(t, lint, s, "rego_policy")
}
