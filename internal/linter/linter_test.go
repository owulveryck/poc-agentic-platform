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
			{
				ADRID:  "ADR-090",
				Title:  "Design tokens are the canonical source of visual style",
				Nature: "amplifier",
				Enforcement: adr.Enforcement{
					Mode:     "programmatic",
					PolicyID: "design_tokens_referenced",
					RegoFile: "ADR-090.rego",
				},
			},
			{
				ADRID:  "ADR-110",
				Title:  "Integrate shared capabilities through the cataloged service",
				Nature: "amplifier",
				Enforcement: adr.Enforcement{
					Mode:     "programmatic",
					PolicyID: "use_cataloged_services",
					RegoFile: "ADR-110.rego",
				},
			},
			{
				ADRID:  "ADR-120",
				Title:  "Governance artifacts are immutable from within agent sessions",
				Nature: "amplifier",
				Enforcement: adr.Enforcement{
					Mode:     "programmatic",
					PolicyID: "governance_artifacts_immutable",
					RegoFile: "ADR-120.rego",
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

func TestGoTestEncodedWithAgentToolNamesPasses(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// A stock coding agent expresses steps with its own tool names: the test
	// step arrives as tool "Bash" with a "go test" action, not as tool "go-test".
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "Create SekaClient", Tool: "Write", Targets: []string{"internal/payment/seka.go"}},
		{ID: "s2", Action: "go test ./internal/payment/...", Tool: "Bash", Targets: []string{"internal/payment/"}},
	})
	for _, v := range lint.Validate(p) {
		if v.PolicyID == "go_tests_present" {
			t.Fatalf("go test encoded as a Bash action should satisfy go_tests_present, got %v", v)
		}
	}
}

func TestMigrationEncodedAsTargetPasses(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// The migration arrives as a file creation under migrations/, not as the
	// canonical db-migration-generator tool.
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "Write the payment_methods migration", Tool: "Write", Targets: []string{"migrations/001_stripe.sql"}},
		{ID: "s2", Action: "alter schema usage", Tool: "patch_code", Targets: []string{"db/schema.sql"}},
		{ID: "s3", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	for _, v := range lint.Validate(p) {
		if v.PolicyID == "db_migration_precedes_code" {
			t.Fatalf("migration encoded as a migrations/ target should satisfy the policy, got %v", v)
		}
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

func TestUIPlanWithoutTokensReadIsRejected(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "write landing page", Tool: "Write", Targets: []string{"index.html"}},
		{ID: "s2", Action: "write page styles", Tool: "Write", Targets: []string{"style.css"}},
		{ID: "s3", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	violations := lint.Validate(p)
	found := false
	for _, v := range violations {
		if v.PolicyID == "design_tokens_referenced" {
			found = true
			if v.Nature != Amplifier {
				t.Errorf("design_tokens_referenced should be tagged amplifier, got %s", v.Nature)
			}
			if !strings.Contains(v.Message, "design/tokens.css") {
				t.Errorf("violation message should name the tokens file, got %q", v.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected design_tokens_referenced violation, got %v", violations)
	}
}

func TestUIPlanReadingTokensPasses(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "read design tokens", Tool: "Read", Targets: []string{"design/tokens.css"}},
		{ID: "s2", Action: "write landing page", Tool: "Write", Targets: []string{"index.html"}},
		{ID: "s3", Action: "write page styles", Tool: "Write", Targets: []string{"style.css"}},
		{ID: "s4", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	for _, v := range lint.Validate(p) {
		if v.PolicyID == "design_tokens_referenced" {
			t.Fatalf("a plan that reads design/tokens.css should satisfy the policy, got %v", v)
		}
	}
}

func hasPolicy(vs []Violation, id string) bool {
	for _, v := range vs {
		if v.PolicyID == id {
			return true
		}
	}
	return false
}

func TestArtifactViewRejectsRawColorsAndButtonRules(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reject := []struct {
		name    string
		content string
	}{
		{"raw hex", ".a{color:#F0F;}"},
		{"var fallback with raw hex", ".a{color:var(--x, #F0F);}"},
		{"rgb function", ".a{color:rgb(1,2,3);}"},
		{"rgba function", ".a{color:rgba(1,2,3,.5);}"},
		{"hsl function", ".a{color:hsl(1,2%,3%);}"},
		{"named color teal", ".a{color:teal;}"},
		{"named color fuchsia", ".a{color: fuchsia;}"},
		{"named color aqua", ".a{color:aqua;}"},
		{"button pseudo-class", "button:hover{padding:4px;}"},
		{"button combinator", "button > span{padding:4px;}"},
		{"btn class", ".btn{padding:4px;}"},
	}
	for _, tc := range reject {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			vs := lint.EvaluateArtifact(Artifact{Path: "index.css", Content: tc.content})
			if !hasPolicy(vs, "design_tokens_referenced") {
				t.Fatalf("expected design_tokens_referenced violation for %q, got %v", tc.content, vs)
			}
		})
	}

	allow := []struct {
		name    string
		path    string
		content string
	}{
		{"token var", "index.css", ".a{color:var(--color-primary);}"},
		{"css keyword", "index.css", ".a{color:transparent;border-color:currentColor;}"},
		{"tokens file exempt", "design/tokens.css", ":root{--color-primary:#0af;} button{padding:4px;}"},
		{"non-ui file", "server.go", `x := "#F0F"`},
	}
	for _, tc := range allow {
		t.Run("allow/"+tc.name, func(t *testing.T) {
			vs := lint.EvaluateArtifact(Artifact{Path: tc.path, Content: tc.content})
			if hasPolicy(vs, "design_tokens_referenced") {
				t.Fatalf("expected no design_tokens violation for %s %q, got %v", tc.path, tc.content, vs)
			}
		})
	}
}

func TestArtifactViewDoesNotFirePlanRules(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// A clean UI artifact must not trip any plan-altitude policy (e.g.
	// go_tests_present, which has no step context in the artifact view).
	vs := lint.EvaluateArtifact(Artifact{Path: "index.css", Content: ".a{color:var(--color-primary);}"})
	if len(vs) != 0 {
		t.Fatalf("expected no violations for a clean artifact, got %v", vs)
	}
}

func TestChangesetViewChecksEveryFile(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// A clean file plus a raw-color file: the changeset must be rejected.
	cs := Changeset{Files: []Artifact{
		{Path: "ok.css", Content: ".a{color:var(--color-primary)}"},
		{Path: "bad.css", Content: ".b{color:#abc}"},
	}}
	if !hasPolicy(lint.EvaluateChangeset(cs), "design_tokens_referenced") {
		t.Fatal("expected a raw-color file in the changeset to be rejected")
	}
	// An all-clean changeset passes.
	clean := Changeset{Files: []Artifact{{Path: "ok.css", Content: ".a{color:var(--color-primary)}"}}}
	if len(lint.EvaluateChangeset(clean)) != 0 {
		t.Fatalf("expected a clean changeset to pass, got %v", lint.EvaluateChangeset(clean))
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

// TestADR110ArtifactRejectsForbiddenProvider verifies the content-altitude
// enforcement of ADR-110: a forbidden provider marker in written code is denied.
func TestADR110ArtifactRejectsForbiddenProvider(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := map[string]string{
		"stripe sdk import":  "package pay\nimport \"github.com/stripe/stripe-go/v76\"\n",
		"stripe api host":    "package pay\nconst url = \"https://api.stripe.com/v1/charges\"\n",
		"legacy mailer host": "package notif\nconst host = \"legacy-mailer.internal:25\"\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			vs := lint.EvaluateArtifact(Artifact{Path: "internal/pay/client.go", Content: content})
			if !hasPolicy(vs, "use_cataloged_services") {
				t.Fatalf("expected use_cataloged_services violation, got %v", vs)
			}
		})
	}
}

// TestADR110ArtifactAllowsSanctionedService confirms code calling the
// recommended gateway endpoint passes.
func TestADR110ArtifactAllowsSanctionedService(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content := "package pay\nconst gateway = \"http://localhost:9120/v1/charges\"\n"
	vs := lint.EvaluateArtifact(Artifact{Path: "internal/pay/client.go", Content: content})
	if hasPolicy(vs, "use_cataloged_services") {
		t.Fatalf("sanctioned gateway call should pass, got %v", vs)
	}
}

// TestADR110ChangesetRejectsForbiddenProvider covers the apply-time altitude.
func TestADR110ChangesetRejectsForbiddenProvider(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cs := Changeset{Files: []Artifact{
		{Path: "internal/pay/client.go", Content: "import \"github.com/stripe/stripe-go/v76\""},
	}}
	if !hasPolicy(lint.EvaluateChangeset(cs), "use_cataloged_services") {
		t.Fatalf("expected changeset rejection for stripe SDK")
	}
}

// TestADR110PlanRejectsNamingStripe covers the plan-view nudge.
func TestADR110PlanRejectsNamingStripe(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "integrate Stripe SDK for checkout", Tool: "patch_code", Targets: []string{"internal/pay/client.go"}},
		{ID: "s2", Action: "go test ./...", Tool: "go-test", Targets: []string{"internal/pay/client_test.go"}},
	})
	if !hasPolicy(lint.Validate(p), "use_cataloged_services") {
		t.Fatalf("expected plan naming Stripe to be flagged")
	}
}

// TestADR120RejectsWriteToTokensCSS covers the plan-altitude enforcement of
// ADR-120: an agent may not modify the canonical design-tokens file, which
// ADR-090 exempts from its artifact-altitude check. This closes the
// "extending the palette" bypass observed live during tutorial 14.
func TestADR120RejectsWriteToTokensCSS(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "recolor the palette", Tool: "Write", Targets: []string{"design/tokens.css"}},
		{ID: "s2", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	violations := lint.Validate(p)
	found := false
	for _, v := range violations {
		if v.PolicyID == "governance_artifacts_immutable" {
			found = true
			if v.Nature != Amplifier {
				t.Errorf("governance_artifacts_immutable should be tagged amplifier, got %s", v.Nature)
			}
			if !strings.Contains(v.Message, "design/tokens.css") {
				t.Errorf("violation message should name the target, got %q", v.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected governance_artifacts_immutable violation, got %v", violations)
	}
}

// TestADR120RejectsWriteToSkillDir covers the prefix-match arm: any write
// under .claude/skills/ or .agents/skills/ is refused. A skill body an agent
// executes under must not be rewritable by that same agent.
func TestADR120RejectsWriteToSkillDir(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "rewrite the design-system skill", Tool: "Edit", Targets: []string{".claude/skills/design-system/SKILL.md"}},
		{ID: "s2", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	if !hasPolicy(lint.Validate(p), "governance_artifacts_immutable") {
		t.Fatalf("expected governance_artifacts_immutable violation for a write under .claude/skills/, got %v", lint.Validate(p))
	}
}

// TestADR120AllowsReadOfTokensCSS covers the negative case: reading
// design/tokens.css is not only allowed, it is required by ADR-090's own
// plan-view rule. ADR-120 must not spuriously fire for read steps.
func TestADR120AllowsReadOfTokensCSS(t *testing.T) {
	lint, err := New(testStore(), "testdata")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := basePlan([]plan.Step{
		{ID: "s1", Action: "read the canonical palette", Tool: "Read", Targets: []string{"design/tokens.css"}},
		{ID: "s2", Action: "write landing page", Tool: "Write", Targets: []string{"index.html"}},
		{ID: "s3", Action: "go test ./...", Tool: "go-test", Targets: []string{"tests/x_test.go"}},
	})
	if hasPolicy(lint.Validate(p), "governance_artifacts_immutable") {
		t.Fatalf("Read step on design/tokens.css should NOT trigger ADR-120, got %v", lint.Validate(p))
	}
}
