package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/owulveryck/poc-agentic-platform/internal/linter"
)

// conflictThreshold is the number of CONSECUTIVE plan rejections with an
// identical violation policy-id set after which the gateway stops answering
// "fix the violations and resubmit" and escalates with POLICY_CONFLICT.
//
// Rationale: an agent that is genuinely iterating changes the violation set
// — fixed rules drop out, new ones may appear. A violation set that is
// byte-for-byte stable across consecutive submissions is a livelock
// signature: either two policies are mutually unsatisfiable for this intent
// (a skill companion requiring what an ADR forbids), or the legal plan
// shape is not reachable from the agent's approach. Both are a human
// decision, and telling the model to "fix and resubmit" a fourth time is
// dishonest guidance. This detects the *symptom* (livelock) — general
// unsatisfiability of a Rego corpus is undecidable and not claimed.
const conflictThreshold = 3

// conflictDetector tracks, per session, how many consecutive lock_in_plan
// rejections carried an identical violation policy-id set. It is
// deliberately simple, deterministic state: same inputs, same escalation.
type conflictDetector struct {
	mu      sync.Mutex
	streaks map[string]*rejectionStreak
}

type rejectionStreak struct {
	key   string // canonical (sorted, deduped) policy-id set of the last rejection
	count int
}

func newConflictDetector() *conflictDetector {
	return &conflictDetector{streaks: make(map[string]*rejectionStreak)}
}

// observeRejection records one rejection and returns the current streak
// length for this exact violation set. A different set resets the streak —
// the agent is making progress (or at least hitting a different wall).
func (d *conflictDetector) observeRejection(sessionID string, policyIDs []string) int {
	key := strings.Join(policyIDs, "\x00")
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.streaks[sessionID]
	if s == nil || s.key != key {
		d.streaks[sessionID] = &rejectionStreak{key: key, count: 1}
		return 1
	}
	s.count++
	return s.count
}

// observeSuccess clears the session's streak: a locked plan proves a legal
// shape exists.
func (d *conflictDetector) observeSuccess(sessionID string) {
	d.mu.Lock()
	delete(d.streaks, sessionID)
	d.mu.Unlock()
}

// violationPolicyIDs returns the sorted, deduplicated policy ids of a
// violation list — the canonical identity of "which wall the plan hit".
func violationPolicyIDs(vs []linter.Violation) []string {
	ids := make([]string, 0, len(vs))
	for _, v := range vs {
		ids = append(ids, v.PolicyID)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// builtinPolicyIDs are the rules synthesized by the linter itself rather
// than loaded from the ADR corpus or a skill companion.
var builtinPolicyIDs = map[string]bool{
	"scope_breadth_cap": true,
	"unknown_skill":     true,
	"linter_eval_error": true,
}

// policySources classifies each policy id by its owning corpus, so the
// escalation names who must be in the room: "adr" (the ADR corpus loaded
// from -adr), "built-in" (the linter's own rules), or "skill" (a companion
// SKILL.rego, operator- or session-provided).
func policySources(lint *linter.Linter, ids []string) map[string]string {
	sources := make(map[string]string, len(ids))
	for _, id := range ids {
		switch {
		case builtinPolicyIDs[id]:
			sources[id] = "built-in"
		default:
			if _, ok := lint.Registry[id]; ok {
				sources[id] = "adr"
			} else {
				sources[id] = "skill"
			}
		}
	}
	return sources
}

// appendEscalation appends one JSON record to the escalation log (JSONL,
// 0600, created on first use). The log is the capitalization feedback loop:
// every record is a conflict a human resolved (or must resolve), and the
// resolution belongs back in the corpus so the same conflict cannot recur.
// Logging failures are logged but never fail the HTTP response — the 409
// block itself is the enforcement; the log is the paper trail.
func appendEscalation(path string, record any) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		log.Printf("escalation log: %v", err)
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("escalation log: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	if err := json.NewEncoder(f).Encode(record); err != nil {
		log.Printf("escalation log: %v", err)
	}
}
