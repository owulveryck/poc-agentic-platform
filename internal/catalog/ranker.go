package catalog

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/owulveryck/poc-agentic-platform/internal/policy"
)

// Verdict is one ranking decision emitted by the catalog policy for a candidate
// service. It is the shape the Rego rule set (data.ppg.catalog.verdict) must
// return, and what the Ranker decodes.
type Verdict struct {
	// ServiceID identifies the candidate the verdict applies to.
	ServiceID string `json:"service_id"`
	// Allow reports whether the service may be recommended (false for
	// deprecated/forbidden).
	Allow bool `json:"allow"`
	// Score orders allowed candidates; higher wins. Ignored when Allow is false.
	Score int `json:"score"`
	// Reason is the human-readable justification surfaced to the agent.
	Reason string `json:"reason"`
}

// Ranked pairs a service with the policy verdict about it, after sorting.
type Ranked struct {
	Service Service
	Verdict Verdict
}

// Ranker applies the org's ranking policy (policy-as-code) to a candidate set.
type Ranker struct {
	eval *policy.Evaluator
}

// NewRanker compiles the ranking policy from every *.rego file in policyDir over
// the query data.ppg.catalog.verdict. It errors when the directory holds no
// policy, so a missing ranking corpus fails fast at startup rather than silently
// denying every service.
func NewRanker(policyDir string) (*Ranker, error) {
	paths, err := filepath.Glob(filepath.Join(policyDir, "*.rego"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("catalog: no ranking policy (*.rego) found in %s", policyDir)
	}
	eval, err := policy.Prepare("data.ppg.catalog.verdict", paths)
	if err != nil {
		return nil, err
	}
	return &Ranker{eval: eval}, nil
}

// rankInput is the document the ranking policy evaluates.
type rankInput struct {
	Capability        string    `json:"capability"`
	RepositoryContext any       `json:"repository_context,omitempty"`
	Candidates        []Service `json:"candidates"`
}

// Rank evaluates the policy over the candidates and returns them ordered:
// allowed services first (highest score, then lowest tier), then denied ones.
// It fails closed — a candidate the policy returns no verdict for is treated as
// denied — so a policy gap never silently blesses a service.
func (r *Ranker) Rank(capability string, repoCtx any, candidates []Service) ([]Ranked, error) {
	verdicts, err := policy.Eval[Verdict](r.eval, rankInput{
		Capability:        capability,
		RepositoryContext: repoCtx,
		Candidates:        candidates,
	})
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Verdict, len(verdicts))
	for _, v := range verdicts {
		byID[v.ServiceID] = v
	}
	ranked := make([]Ranked, 0, len(candidates))
	for _, svc := range candidates {
		v, ok := byID[svc.ServiceID]
		if !ok {
			v = Verdict{
				ServiceID: svc.ServiceID,
				Allow:     false,
				Reason:    "no ranking verdict from policy (fail-closed).",
			}
		}
		ranked = append(ranked, Ranked{Service: svc, Verdict: v})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.Verdict.Allow != b.Verdict.Allow {
			return a.Verdict.Allow // allowed before denied
		}
		if a.Verdict.Score != b.Verdict.Score {
			return a.Verdict.Score > b.Verdict.Score
		}
		if a.Service.Tier != b.Service.Tier {
			return a.Service.Tier < b.Service.Tier
		}
		return a.Service.ServiceID < b.Service.ServiceID
	})
	return ranked, nil
}
