package enrich

import (
	"reflect"
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

func testStore() *adr.Store {
	return &adr.Store{Invariants: []adr.Invariant{
		{
			ADRID:          "ADR-001",
			Nature:         "amplifier",
			ScopeSelectors: []string{"payment"},
			InvariantText:  "All payment flows go through the payments gateway.",
		},
		{
			ADRID:           "ADR-002",
			Nature:          "compensatory",
			SunsetCondition: "model handles X reliably",
			ScopeSelectors:  []string{"payment", "checkout"},
			InvariantText:   "Enumerate frozen legacy files explicitly.",
		},
		{
			ADRID:          "ADR-003",
			Nature:         "amplifier",
			ScopeSelectors: []string{"frontend"},
			InvariantText:  "Use design tokens.",
		},
	}}
}

func TestEnrichRetrievesMatchingInvariants(t *testing.T) {
	resp := Enrich(testStore(), "add a payment method to checkout", plan.RepoContext{})

	if resp.Status != "CONTEXT_ENRICHED" {
		t.Fatalf("status = %q, want CONTEXT_ENRICHED", resp.Status)
	}
	if got, want := resp.AmplifierContext.SourceADRs, []string{"ADR-001", "ADR-002"}; !reflect.DeepEqual(got, want) {
		t.Errorf("source_adrs = %v, want %v", got, want)
	}
	if len(resp.AmplifierContext.ArchitecturalInvariants) != 2 {
		t.Fatalf("got %d invariants, want 2", len(resp.AmplifierContext.ArchitecturalInvariants))
	}
	if resp.AmplifierContext.ArchitecturalInvariants[0].Invariant == "" {
		t.Error("invariant text must be carried through")
	}
}

func TestEnrichSurfacesCompensatoryScaffolding(t *testing.T) {
	resp := Enrich(testStore(), "work on payment", plan.RepoContext{})

	if len(resp.CompensatoryScaffolding) != 1 {
		t.Fatalf("got %d scaffolding entries, want 1", len(resp.CompensatoryScaffolding))
	}
	sc := resp.CompensatoryScaffolding[0]
	if sc.ADRID != "ADR-002" || sc.SunsetCondition != "model handles X reliably" {
		t.Errorf("scaffolding = %+v, want ADR-002 with its sunset condition", sc)
	}
}

func TestEnrichNoMatchReturnsEmptyNonNilSlices(t *testing.T) {
	resp := Enrich(testStore(), "something entirely unrelated", plan.RepoContext{})

	// The JSON contract promises arrays, never null.
	if resp.AmplifierContext.ArchitecturalInvariants == nil ||
		resp.AmplifierContext.SourceADRs == nil ||
		resp.CompensatoryScaffolding == nil {
		t.Fatal("empty response must keep non-nil slices (JSON arrays)")
	}
	if len(resp.AmplifierContext.ArchitecturalInvariants) != 0 {
		t.Errorf("expected no invariants, got %d", len(resp.AmplifierContext.ArchitecturalInvariants))
	}
}
