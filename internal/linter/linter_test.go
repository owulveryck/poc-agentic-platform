package linter

import (
	"strings"
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

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
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "edit router", Tool: "patch_code", Targets: []string{"internal/payment/router.go"}},
	})
	violations := Validate(p)
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
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "generate migration", Tool: "db-migration-generator", Targets: []string{"migrations/001_seka.sql"}},
		{ID: "s2", Action: "edit router", Tool: "patch_code", Targets: []string{"internal/payment/router.go"}},
		{ID: "s3", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/integration_payment_test.go"}},
	})
	if violations := Validate(p); len(violations) != 0 {
		t.Fatalf("expected no violations, got %v", violations)
	}
}

func TestFrozenFileIsRejected(t *testing.T) {
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "edit legacy", Tool: "patch_code", Targets: []string{"internal/old_payment.go"}},
		{ID: "s2", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	violations := Validate(p)
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

func TestDBChangeRequiresMigration(t *testing.T) {
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "alter table", Tool: "patch_code", Targets: []string{"db/schema.sql"}},
		{ID: "s2", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	violations := Validate(p)
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
