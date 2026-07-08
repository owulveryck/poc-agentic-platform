package debt

import (
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/linter"
)

// testRegistry mirrors the production ADR policies for test assertions.
var testRegistry = map[string]linter.PolicyMeta{
	"go_tests_present": {
		Nature:    linter.Amplifier,
		Rationale: "SDLC invariant: the tests must exist, whoever writes them.",
	},
	"db_migration_precedes_code": {
		Nature:    linter.Amplifier,
		Rationale: "Ordering invariant, true whatever the model.",
	},
	"external_call_via_proxy": {
		Nature:    linter.Amplifier,
		Rationale: "Organizational security constraint, enforced declaratively via ADR-042.",
	},
	"explicit_frozen_files_enumeration": {
		Nature:          linter.Compensatory,
		Rationale:       "Exhaustive enumeration needed as long as the model cannot infer deprecated legacy code on its own.",
		SunsetCondition: "Model honors '@deprecated' semantically on >95% of an internal benchmark.",
	},
}

func TestDebtRatioStaysUnderThreshold(t *testing.T) {
	report := Compute(testRegistry)
	if report.TransitionDebtRatio >= 0.5 {
		t.Fatalf("too much compensatory scaffolding: %+v", report)
	}
}

func TestEveryCompensatoryArtifactHasASunsetCondition(t *testing.T) {
	report := Compute(testRegistry)
	for _, pending := range report.PendingSunsets {
		if pending.SunsetCondition == "" {
			t.Errorf("compensatory artifact without a sunset condition: %s", pending.ArtifactID)
		}
	}
	if report.CompensatoryCount != len(report.PendingSunsets) {
		t.Errorf("every compensatory artifact must be listed in pending_sunsets")
	}
}
