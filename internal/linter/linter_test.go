package linter

import (
	"strings"
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

// testStore returns a minimal ADR store with the three programmatic policies,
// pointing at .rego files in the testdata/ directory.
func testStore() *adr.Store {
	return &adr.Store{
		Invariants: []adr.Invariant{
			{
				ADRID:  "ADR-060",
				Title:  "Test suite required for Go stacks",
				Nature: "amplifier",
				Enforcement: adr.Enforcement{
					Mode:     "programmatic",
					PolicyID: "go_tests_present",
					RegoFile: "ADR-060.rego",
				},
			},
			{
				ADRID:  "ADR-051",
				Title:  "Schema migrations precede code changes",
				Nature: "amplifier",
				Enforcement: adr.Enforcement{
					Mode:     "programmatic",
					PolicyID: "db_migration_precedes_code",
					RegoFile: "ADR-051.rego",
				},
			},
			{
				ADRID:           "ADR-070",
				Title:           "Frozen legacy paths enumeration",
				Nature:          "compensatory",
				SunsetCondition: "Model honors '@deprecated' semantically on >95% of an internal benchmark.",
				Enforcement: adr.Enforcement{
					Mode:     "programmatic",
					PolicyID: "explicit_frozen_files_enumeration",
					RegoFile: "ADR-070.rego",
				},
			},
		},
	}
}

func basePlan(steps []plan.Step) *plan.Plan {
	return &plan.Plan{
		SessionID: "11111111-1111-1111-1111-111111111111",
		Intent:    "Add the Seka payment method",
		RepositoryContext: plan.RepoContext{
			Name:      "checkout-service",
			TechStack: []string{"Go"},
		},
		Steps: steps,
	}
}

func TestGoPlanWithoutTestsIsRejected(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "edit router", Tool: "patch_code", Targets: []string{"internal/payment/router.go"}},
	})
	violations := lint.Validate(p)
	if len(violations) == 0 {
		t.Fatal("expected a violation for a Go plan without tests")
	}
	found := false
	for _, v := range violations {
		if v.PolicyID == "go_tests_present" {
			found = true
			if v.Nature != Amplifier {
				t.Errorf("go_tests_present should be tagged amplifier, got %s", v.Nature)
			}
		}
	}
	if !found {
		t.Fatalf("expected go_tests_present violation, got %v", violations)
	}
}

func TestConformingPlanPasses(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "generate migration", Tool: "db-migration-generator", Targets: []string{"migrations/001_seka.sql"}},
		{ID: "s2", Action: "edit router", Tool: "patch_code", Targets: []string{"internal/payment/router.go"}},
		{ID: "s3", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/integration_payment_test.go"}},
	})
	if violations := lint.Validate(p); len(violations) != 0 {
		t.Fatalf("expected no violations, got %v", violations)
	}
}

func TestFrozenFileIsRejected(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "edit legacy", Tool: "patch_code", Targets: []string{"internal/old_payment.go"}},
		{ID: "s2", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	violations := lint.Validate(p)
	found := false
	for _, v := range violations {
		if v.PolicyID == "explicit_frozen_files_enumeration" {
			found = true
			if v.Nature != Compensatory {
				t.Errorf("frozen files enumeration should be tagged compensatory, got %s", v.Nature)
			}
			if !strings.Contains(v.Message, "old_payment") {
				t.Errorf("violation message should name the file, got %q", v.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected frozen-file violation, got %v", violations)
	}
}

func TestUndecodableViolationFailsClosed(t *testing.T) {
	store := &adr.Store{
		Invariants: []adr.Invariant{
			{
				ADRID:  "BAD-001",
				Title:  "Malformed policy output",
				Nature: "amplifier",
				Enforcement: adr.Enforcement{
					Mode:     "programmatic",
					PolicyID: "malformed_shape",
					RegoFile: "BAD-001.rego",
				},
			},
		},
	}
	lint, err := New(store, "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	violations := lint.Validate(basePlan([]plan.Step{
		{ID: "s1", Action: "noop", Tool: "patch_code", Targets: []string{"x.txt"}},
	}))
	if len(violations) == 0 {
		t.Fatal("expected a linter_eval_error violation when the OPA result cannot be decoded (fail closed), got none")
	}
	if violations[0].PolicyID != "linter_eval_error" {
		t.Fatalf("expected linter_eval_error, got %v", violations)
	}
}

func TestDBChangeRequiresMigration(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "alter table", Tool: "patch_code", Targets: []string{"db/schema.sql"}},
		{ID: "s2", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	violations := lint.Validate(p)
	found := false
	for _, v := range violations {
		if v.PolicyID == "db_migration_precedes_code" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected db_migration_precedes_code violation, got %v", violations)
	}
}
