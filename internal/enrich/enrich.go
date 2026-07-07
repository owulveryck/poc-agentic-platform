// Package enrich builds the amplifier context returned to the agent before it
// plans.
//
// Amplifier / declarative: no business pattern is hard-coded here. The
// function retrieves the semantic invariants declared by the architects in
// the ADR store and lets the model reason over them — the smarter the model,
// the better the exploitation.
package enrich

import (
	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

// InvariantRef is one invariant surfaced to the agent.
type InvariantRef struct {
	ADRID     string `json:"adr_id"`
	Invariant string `json:"invariant"`
}

// Scaffolding exposes the compensatory debt mobilized for this intent, so the
// consumer sees which parts of its context are scheduled to disappear.
type Scaffolding struct {
	ADRID           string `json:"adr_id"`
	SunsetCondition string `json:"sunset_condition"`
}

// Response is the payload of POST /enrich.
type Response struct {
	Status           string `json:"status"`
	AmplifierContext struct {
		ArchitecturalInvariants []InvariantRef `json:"architectural_invariants"`
		SourceADRs              []string       `json:"source_adrs"`
	} `json:"amplifier_context"`
	CompensatoryScaffolding []Scaffolding `json:"compensatory_scaffolding"`
}

// Enrich retrieves the invariants relevant to the intent.
func Enrich(store *adr.Store, intent string, _ plan.RepoContext) Response {
	resp := Response{
		Status:                  "CONTEXT_ENRICHED",
		CompensatoryScaffolding: []Scaffolding{},
	}
	resp.AmplifierContext.ArchitecturalInvariants = []InvariantRef{}
	resp.AmplifierContext.SourceADRs = []string{}
	for _, inv := range store.Retrieve(intent) {
		resp.AmplifierContext.ArchitecturalInvariants = append(
			resp.AmplifierContext.ArchitecturalInvariants,
			InvariantRef{ADRID: inv.ADRID, Invariant: inv.InvariantText},
		)
		resp.AmplifierContext.SourceADRs = append(resp.AmplifierContext.SourceADRs, inv.ADRID)
		if inv.Nature == "compensatory" {
			resp.CompensatoryScaffolding = append(resp.CompensatoryScaffolding, Scaffolding{
				ADRID:           inv.ADRID,
				SunsetCondition: inv.SunsetCondition,
			})
		}
	}
	return resp
}
