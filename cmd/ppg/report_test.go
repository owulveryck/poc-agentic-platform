package main

import (
	"slices"
	"testing"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/journal"
)

// fixtureEvents builds a stream covering the signals ppg report exists for:
// session A is asked twice then locks, re-locks later, and has mixed guard
// verdicts; session B shows bypass indicators (session_mismatch block, plan
// substitution) and a fail-closed verify.
func fixtureEvents() []journal.Event {
	t0 := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }
	return []journal.Event{
		{Time: at(0), Name: journal.EventSessionStart, SessionID: "aaaa", Component: "ppg-guard"},
		{Time: at(1), Name: journal.EventIntentDeclared, SessionID: "aaaa", Component: "ppg-mcp-server"},
		{Time: at(2), Name: journal.EventPlanRejected, Severity: "WARN", SessionID: "aaaa", Component: "ppg",
			Attrs: map[string]any{"policy_ids": []any{"go_tests_present", "design_tokens_referenced"}}},
		{Time: at(3), Name: journal.EventPlanRejected, Severity: "WARN", SessionID: "aaaa", Component: "ppg",
			Attrs: map[string]any{"policy_ids": []any{"go_tests_present"}}},
		{Time: at(4), Name: journal.EventPlanLocked, SessionID: "aaaa", Component: "ppg"},
		{Time: at(5), Name: journal.EventGuardAllow, SessionID: "aaaa", Component: "ppg-guard"},
		{Time: at(6), Name: journal.EventGuardBlock, Severity: "WARN", SessionID: "aaaa", Component: "ppg-guard",
			Attrs: map[string]any{"reason_code": "out_of_plan_scope"}},
		{Time: at(7), Name: journal.EventPlanLocked, SessionID: "aaaa", Component: "ppg"}, // re-lock
		{Time: at(8), Name: journal.EventChangesetOK, SessionID: "aaaa", Component: "ppg"},

		{Time: at(10), Name: journal.EventGuardBlock, Severity: "WARN", SessionID: "bbbb", Component: "ppg-guard",
			Attrs: map[string]any{"reason_code": "session_mismatch"}},
		{Time: at(11), Name: journal.EventPlanSubstitution, Severity: "WARN", SessionID: "bbbb", Component: "ppg"},
		{Time: at(12), Name: journal.EventVerifyRun, Severity: "ERROR", SessionID: "bbbb", Component: "ppg-verify",
			Attrs: map[string]any{"outcome": "error", "status": "SERVER_UNREACHABLE"}},
	}
}

func TestAggregate(t *testing.T) {
	data := aggregate(fixtureEvents())

	if data.EventCount != 12 {
		t.Fatalf("EventCount = %d, want 12", data.EventCount)
	}
	if len(data.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(data.Sessions))
	}

	a := data.Sessions[0]
	if a.SessionID != "aaaa" {
		t.Fatalf("first session = %s, want aaaa", a.SessionID)
	}
	if a.Turns != 2 || a.Intents != 1 || a.PlanRejected != 2 || a.MaxRejectRun != 2 {
		t.Fatalf("session aaaa counters wrong: %+v", a)
	}
	if a.Blocks != 1 || a.Allows != 1 || a.Verifies != 1 {
		t.Fatalf("session aaaa verdicts wrong: %+v", a)
	}
	for _, f := range []string{"asked-twice", "re-locked"} {
		if !slices.Contains(a.Flags, f) {
			t.Fatalf("session aaaa should be flagged %q, got %v", f, a.Flags)
		}
	}

	b := data.Sessions[1]
	for _, f := range []string{"bypass?", "substitution", "unverified-window"} {
		if !slices.Contains(b.Flags, f) {
			t.Fatalf("session bbbb should be flagged %q, got %v", f, b.Flags)
		}
	}

	if data.BlocksByReason["out_of_plan_scope"] != 1 || data.BlocksByReason["session_mismatch"] != 1 {
		t.Fatalf("BlocksByReason wrong: %v", data.BlocksByReason)
	}
	if data.Substitutions != 1 {
		t.Fatalf("Substitutions = %d, want 1", data.Substitutions)
	}

	// go_tests_present was violated twice, design_tokens_referenced once:
	// the ranking must lead with the former.
	if len(data.TopInvariants) != 2 || data.TopInvariants[0].PolicyID != "go_tests_present" || data.TopInvariants[0].Count != 2 {
		t.Fatalf("TopInvariants wrong: %+v", data.TopInvariants)
	}
	if !slices.Contains(data.TopInvariants[0].Events, journal.EventPlanRejected) {
		t.Fatalf("invariant source events wrong: %+v", data.TopInvariants[0])
	}
}

// TestAggregateSortsOutOfOrderEvents proves the reject-run logic survives the
// interleaved, slightly out-of-order appends of five independent writers.
func TestAggregateSortsOutOfOrderEvents(t *testing.T) {
	evts := fixtureEvents()
	// Shuffle deterministically: reverse.
	slices.Reverse(evts)
	data := aggregate(evts)
	i := slices.IndexFunc(data.Sessions, func(s sessionReport) bool { return s.SessionID == "aaaa" })
	if i < 0 || data.Sessions[i].MaxRejectRun != 2 || data.Sessions[i].Turns != 2 {
		t.Fatalf("session aaaa after reorder: %+v", data.Sessions)
	}
}
