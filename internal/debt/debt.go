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
	// ArtifactID identifies the compensatory artifact (matches a linter
	// registry key or a translator constant name).
	ArtifactID string `json:"artifact_id"`
	// SunsetCondition is the measurable condition under which this artifact
	// can be removed.
	SunsetCondition string `json:"sunset_condition"`
}

// Report is the payload of GET /debt_report.
type Report struct {
	// TotalArtifacts is the number of tracked governance artifacts
	// (linter policies + translators).
	TotalArtifacts int `json:"total_artifacts"`
	// CompensatoryCount is the number of artifacts tagged as compensatory.
	CompensatoryCount int `json:"compensatory_count"`
	// AmplifierCount is the number of artifacts tagged as amplifier.
	AmplifierCount int `json:"amplifier_count"`
	// TransitionDebtRatio is CompensatoryCount / TotalArtifacts. Must trend
	// toward 0 as models improve and compensatory artifacts are removed.
	TransitionDebtRatio float64 `json:"transition_debt_ratio"`
	// PendingSunsets lists each compensatory artifact with its sunset condition.
	PendingSunsets []PendingSunset `json:"pending_sunsets"`
	// Health is "OK" when TransitionDebtRatio < 0.3, "DEBT_ALERT" otherwise.
	Health string `json:"health"`
}

// Compute aggregates the tagged policies and translators into a debt report.
// It sets Health to "DEBT_ALERT" when the compensatory ratio reaches 30 % or
// more, signalling that the platform is carrying too much temporary scaffolding.
func Compute(registry map[string]linter.PolicyMeta) Report {
	report := Report{PendingSunsets: []PendingSunset{}}

	for id, meta := range registry {
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
