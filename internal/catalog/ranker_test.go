package catalog

import "testing"

func newTestRanker(t *testing.T) *Ranker {
	t.Helper()
	r, err := NewRanker("testdata")
	if err != nil {
		t.Fatalf("NewRanker: %v", err)
	}
	return r
}

func TestRankPrefersRecommendedOverDeprecated(t *testing.T) {
	r := newTestRanker(t)
	candidates := []Service{
		{ServiceID: "legacy-mailer", Capability: "notification", Status: StatusDeprecated, SupersededBy: "notify-svc", Tier: 5},
		{ServiceID: "notify-svc", Capability: "notification", Status: StatusRecommended, Tier: 1},
	}
	ranked, err := r.Rank("notification", nil, candidates)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if ranked[0].Service.ServiceID != "notify-svc" || !ranked[0].Verdict.Allow {
		t.Fatalf("expected notify-svc recommended first, got %+v", ranked[0])
	}
	if ranked[1].Verdict.Allow {
		t.Errorf("legacy-mailer should be denied (deprecated), got allow")
	}
	if ranked[1].Verdict.Reason == "" || !containsSub(ranked[1].Verdict.Reason, "superseded by notify-svc") {
		t.Errorf("expected superseded reason, got %q", ranked[1].Verdict.Reason)
	}
}

func TestRankDeniesForbidden(t *testing.T) {
	r := newTestRanker(t)
	candidates := []Service{
		{ServiceID: "stripe-direct", Capability: "payment", Status: StatusForbidden, Tier: 1},
		{ServiceID: "payments-gateway", Capability: "payment", Status: StatusRecommended, Tier: 1},
	}
	ranked, err := r.Rank("payment", nil, candidates)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if ranked[0].Service.ServiceID != "payments-gateway" {
		t.Fatalf("expected payments-gateway first, got %s", ranked[0].Service.ServiceID)
	}
	for _, rk := range ranked {
		if rk.Service.ServiceID == "stripe-direct" && rk.Verdict.Allow {
			t.Errorf("stripe-direct should be denied (forbidden)")
		}
	}
}

func TestRankTieBreaksByTier(t *testing.T) {
	r := newTestRanker(t)
	candidates := []Service{
		{ServiceID: "b-svc", Capability: "storage", Status: StatusAllowed, Tier: 3},
		{ServiceID: "a-svc", Capability: "storage", Status: StatusAllowed, Tier: 1},
	}
	ranked, err := r.Rank("storage", nil, candidates)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if ranked[0].Service.ServiceID != "a-svc" {
		t.Errorf("expected lower-tier a-svc first, got %s", ranked[0].Service.ServiceID)
	}
}

func TestRankUnknownStatusFailsClosed(t *testing.T) {
	r := newTestRanker(t)
	candidates := []Service{{ServiceID: "mystery", Capability: "x", Status: "bogus"}}
	ranked, err := r.Rank("x", nil, candidates)
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(ranked) != 1 || ranked[0].Verdict.Allow {
		t.Errorf("unknown status must be denied (fail-closed), got %+v", ranked)
	}
}

func containsSub(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
