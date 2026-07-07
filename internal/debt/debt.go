// Package debt computes the transition-debt report: how much compensatory
// scaffolding the platform still maintains, and under which conditions each
// piece will be removed. The ratio must trend toward zero as models improve —
// it is the health indicator of the platform investment.
package debt

import (
	"github.com/owulveryck/poc-agentic-platform/internal/linter"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/translate"
)

// PendingSunset is one compensatory artifact awaiting removal.
type PendingSunset struct {
	ArtifactID      string `json:"artifact_id"`
	SunsetCondition string `json:"sunset_condition"`
}

// Report is the payload of GET /debt_report.
type Report struct {
	TotalArtifacts      int             `json:"total_artifacts"`
	CompensatoryCount   int             `json:"compensatory_count"`
	AmplifierCount      int             `json:"amplifier_count"`
	TransitionDebtRatio float64         `json:"transition_debt_ratio"`
	PendingSunsets      []PendingSunset `json:"pending_sunsets"`
	Health              string          `json:"health"`
}

// Compute aggregates the tagged policies and translators.
func Compute() Report {
	report := Report{PendingSunsets: []PendingSunset{}}

	for id, meta := range linter.Registry {
		report.TotalArtifacts++
		if meta.Nature == linter.Compensatory {
			report.CompensatoryCount++
			report.PendingSunsets = append(report.PendingSunsets, PendingSunset{
				ArtifactID:      id,
				SunsetCondition: meta.SunsetCondition,
			})
		}
	}

	// The generic raw→JSON translator is compensatory scaffolding too.
	report.TotalArtifacts++
	report.CompensatoryCount++
	report.PendingSunsets = append(report.PendingSunsets, PendingSunset{
		ArtifactID:      "generic_raw_to_json_translator",
		SunsetCondition: translate.GenericSunsetCondition,
	})

	report.AmplifierCount = report.TotalArtifacts - report.CompensatoryCount
	if report.TotalArtifacts > 0 {
		report.TransitionDebtRatio = float64(report.CompensatoryCount) / float64(report.TotalArtifacts)
	}
	report.Health = "OK"
	if report.TransitionDebtRatio >= 0.3 {
		report.Health = "DEBT_ALERT"
	}
	return report
}
