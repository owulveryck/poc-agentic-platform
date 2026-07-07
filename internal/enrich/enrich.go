// Package enrich answers the one question an agent should ask before it
// plans: "here is what I am about to do (the intent) and where (the
// repository): which of the organization's architectural decisions apply?"
//
// Input:  the natural-language intent + the repository context.
// Output: the "amplifier context": the invariants of every ADR whose scope
// selectors match the intent, plus the list of compensatory scaffolding
// mobilized (so the consumer sees which parts of its context are scheduled
// to disappear).
//
// The caller injects the returned invariants into the agent's planning
// context; the agent reasons over them and shapes its plan accordingly.
// In short: a retrieval service over the architecture knowledge base,
// scoped to the intent.
//
// Two deliberate non-goals:
//   - enrich never enforces: it advises. Enforcement happens later and
//     deterministically, in the plan linter at lock_in_plan time.
//   - enrich never returns recipes ("edit file X at line Y"): only semantic
//     invariants. No business pattern is hard-coded here — architects
//     declare both the invariants and their scope selectors in the ADR
//     store (see internal/adr), and the gateway only retrieves them.
//
// Durability: amplifier / declarative. The smarter the model, the better
// it exploits the same invariants.
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
