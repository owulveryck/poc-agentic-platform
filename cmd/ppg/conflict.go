package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/owulveryck/poc-agentic-platform/internal/linter"
)

// conflictThreshold is the number of plan rejections carrying an identical
// violation policy-id set — within one session, consecutive or not — after
// which the validation server stops answering "fix the violations and
// resubmit" and escalates with POLICY_CONFLICT.
//
// Rationale: an agent that is genuinely iterating makes the violation set
// evolve — fixed rules drop out, new ones may appear — and eventually locks
// a plan. Hitting the exact same wall conflictThreshold times without a
// successful lock in between is a livelock signature: either two policies
// are mutually unsatisfiable for this intent (a skill companion requiring
// what an ADR forbids), or the legal plan shape is not reachable from the
// agent's approach. Both are a human decision, and telling the model to
// "fix and resubmit" once more is dishonest guidance. Counting per set
// rather than per consecutive streak means alternating between two plan
// shapes cannot keep the counter at 1 forever. This detects the *symptom*
// (livelock) — general unsatisfiability of a Rego corpus is undecidable and
// not claimed.
const conflictThreshold = 3

// conflictDetector tracks plan-rejection livelocks. Two layers of state:
//
//   - counts: per session, how many rejections carried each violation set.
//     Cleared for a session by a successful lock (a locked plan proves a
//     legal shape exists); pre-escalation state only.
//   - escalated: violation sets that crossed the threshold. Global — once a
//     set is escalated it blocks EVERY session that produces it, so
//     starting a fresh session does not reopen a conflict a human has not
//     resolved. Entries leave this map only through `ppg escalations
//     resolve` (picked up on SIGHUP) — never through agent behavior.
//
// State persists to statePath (JSON, 0600, atomic rename) on every
// mutation, so a server restart does not reset livelock accounting. An
// empty statePath keeps the detector in-memory (tests).
type conflictDetector struct {
	mu        sync.Mutex
	statePath string
	state     conflictState
}

type conflictState struct {
	// Counts maps session id -> canonical violation-set key -> rejections.
	Counts map[string]map[string]int `json:"counts"`
	// Escalated maps conflict id -> the escalated conflict.
	Escalated map[string]*escalatedConflict `json:"escalated"`
}

type escalatedConflict struct {
	PolicyIDs  []string `json:"policy_ids"`
	FirstTS    string   `json:"first_ts"`
	SessionID  string   `json:"session_id"` // session that triggered the escalation
	Rejections int      `json:"rejections"` // total rejections observed for this set, across sessions
}

func newConflictDetector(statePath string) *conflictDetector {
	d := &conflictDetector{
		statePath: statePath,
		state: conflictState{
			Counts:    make(map[string]map[string]int),
			Escalated: make(map[string]*escalatedConflict),
		},
	}
	if statePath == "" {
		return d
	}
	raw, err := os.ReadFile(statePath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return d
	case err != nil:
		log.Printf("conflict state: %v — starting with empty livelock state", err)
		return d
	}
	var loaded conflictState
	if err := json.Unmarshal(raw, &loaded); err != nil {
		log.Printf("conflict state: unreadable %s (%v) — starting with empty livelock state", statePath, err)
		return d
	}
	if loaded.Counts != nil {
		d.state.Counts = loaded.Counts
	}
	if loaded.Escalated != nil {
		d.state.Escalated = loaded.Escalated
	}
	return d
}

// conflictID is the stable identity of a violation set: the first 12 hex
// chars of the SHA-256 of its canonical key. It names the conflict in the
// 409 response, the escalation log, the persisted state, and the `ppg
// escalations` CLI.
func conflictID(setKey string) string {
	sum := sha256.Sum256([]byte(setKey))
	return hex.EncodeToString(sum[:])[:12]
}

// observeRejection records one rejection of this violation set and reports
// where it stands: the rejection count for the set, its conflict id, and
// whether the set is (now or already) escalated. ts is the caller's
// timestamp for a new escalation record.
func (d *conflictDetector) observeRejection(sessionID string, policyIDs []string, ts string) (count int, id string, blocked bool) {
	key := strings.Join(policyIDs, "\x00")
	id = conflictID(key)
	d.mu.Lock()
	defer d.mu.Unlock()

	if esc, ok := d.state.Escalated[id]; ok {
		esc.Rejections++
		d.persistLocked()
		return esc.Rejections, id, true
	}

	perSet := d.state.Counts[sessionID]
	if perSet == nil {
		perSet = make(map[string]int)
		d.state.Counts[sessionID] = perSet
	}
	perSet[key]++
	count = perSet[key]
	if count >= conflictThreshold {
		d.state.Escalated[id] = &escalatedConflict{
			PolicyIDs:  policyIDs,
			FirstTS:    ts,
			SessionID:  sessionID,
			Rejections: count,
		}
		delete(perSet, key)
		if len(perSet) == 0 {
			delete(d.state.Counts, sessionID)
		}
		d.persistLocked()
		return count, id, true
	}
	d.persistLocked()
	return count, id, false
}

// observeSuccess clears the session's pre-escalation counters: a locked
// plan proves a legal shape exists for what the session is attempting.
// Escalated conflicts are NOT touched — they stay blocked for every session
// until a human resolves them (`ppg escalations resolve` + SIGHUP).
func (d *conflictDetector) observeSuccess(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.state.Counts[sessionID]; !ok {
		return
	}
	delete(d.state.Counts, sessionID)
	d.persistLocked()
}

// syncFromDisk re-reads the persisted escalated set, dropping entries a
// human resolved with `ppg escalations resolve` while keeping the live
// in-memory rejection counters. Called from the SIGHUP handler — resolving
// a conflict rides the same ritual as capitalizing the corpus fix itself.
func (d *conflictDetector) syncFromDisk() {
	if d.statePath == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	raw, err := os.ReadFile(d.statePath)
	if errors.Is(err, os.ErrNotExist) {
		d.state.Escalated = make(map[string]*escalatedConflict)
		d.persistLocked()
		return
	}
	if err != nil {
		log.Printf("conflict state: %v — keeping in-memory escalations", err)
		return
	}
	var loaded conflictState
	if err := json.Unmarshal(raw, &loaded); err != nil {
		log.Printf("conflict state: unreadable %s (%v) — keeping in-memory escalations", d.statePath, err)
		return
	}
	if loaded.Escalated == nil {
		loaded.Escalated = make(map[string]*escalatedConflict)
	}
	d.state.Escalated = loaded.Escalated
	d.persistLocked()
}

// resolve removes an escalated conflict from the state and persists — the
// `ppg escalations resolve` path. Returns the removed record, or false when
// the id is not escalated. The running server adopts the removal on SIGHUP
// (syncFromDisk).
func (d *conflictDetector) resolve(id string) (*escalatedConflict, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	esc, ok := d.state.Escalated[id]
	if !ok {
		return nil, false
	}
	delete(d.state.Escalated, id)
	d.persistLocked()
	return esc, true
}

// persistLocked writes the state file atomically (same-directory temp file
// + rename, 0600). Callers hold d.mu. Persistence failures are logged and
// never fail the request — the in-memory block is the enforcement; the file
// is what survives a restart.
func (d *conflictDetector) persistLocked() {
	if d.statePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(d.statePath), 0o700); err != nil {
		log.Printf("conflict state: %v", err)
		return
	}
	raw, err := json.Marshal(d.state)
	if err != nil {
		log.Printf("conflict state: %v", err)
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(d.statePath), ".conflicts-*")
	if err != nil {
		log.Printf("conflict state: %v", err)
		return
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(raw)
	if werr == nil {
		werr = tmp.Chmod(0o600)
	}
	if cerr := tmp.Close(); cerr != nil && werr == nil {
		werr = cerr
	}
	if werr != nil {
		log.Printf("conflict state: %v", werr)
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, d.statePath); err != nil {
		log.Printf("conflict state: %v", err)
		_ = os.Remove(tmpName)
	}
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
// `ppg escalations` reads it. Logging failures are logged but never fail
// the HTTP response — the 409 block itself is the enforcement; the log is
// the paper trail.
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
