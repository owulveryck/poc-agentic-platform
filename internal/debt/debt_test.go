package debt

import "testing"

func TestDebtRatioStaysUnderThreshold(t *testing.T) {
	report := Compute()
	if report.TransitionDebtRatio >= 0.5 {
		t.Fatalf("too much compensatory scaffolding: %+v", report)
	}
}

func TestEveryCompensatoryArtifactHasASunsetCondition(t *testing.T) {
	report := Compute()
	for _, pending := range report.PendingSunsets {
		if pending.SunsetCondition == "" {
			t.Errorf("compensatory artifact without a sunset condition: %s", pending.ArtifactID)
		}
	}
	if report.CompensatoryCount != len(report.PendingSunsets) {
		t.Errorf("every compensatory artifact must be listed in pending_sunsets")
	}
}
