package main

// ppg report — the a-posteriori consumer of the decision-event journal
// (internal/journal). It aggregates <state-root>/events.jsonl (plus rotated
// events-*.jsonl) into the guardrail-improvement signals the journal exists
// for: loop turns per session, plans that had to be asked twice, guard blocks
// by reason, the most-violated invariants, and bypass indicators.
//
//	ppg report [-store-root DIR] [-session ID] [-since 24h] [-json]
//
// Definitions (the same ones the live dashboard computes client-side):
//   - a loop TURN is one ppg.plan.locked — one full pass of the governed
//     intent→plan→act→observe loop;
//   - ASKED-TWICE is a maximal run of ≥2 consecutive ppg.plan.rejected for a
//     session with no ppg.plan.locked in between;
//   - RE-LOCKED is ≥2 ppg.plan.locked in one session (scope grew mid-session);
//   - bypass indicators: any ppg.plan.substitution; guard blocks with
//     reason_code session_mismatch or ticket_rejected; ppg.verify.run with
//     outcome=error (the backstop could not run — an unverified window).
//
// Double-counting rule: blocks are counted from ppg.guard.block only; policy
// ids are attributed from server events only (plan/artifact/changeset
// rejections), which are the only events that carry them.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/journal"
	storepkg "github.com/owulveryck/poc-agentic-platform/internal/store"
)

func runReport(args []string) int {
	fs := flag.NewFlagSet("ppg report", flag.ExitOnError)
	storeRoot := fs.String("store-root", "", "per-machine state root (default $PPG_STORE_ROOT, else $XDG_STATE_HOME/ppg)")
	sessionFilter := fs.String("session", "", "only this session id (prefix match)")
	since := fs.Duration("since", 0, "only events younger than this (e.g. 24h; 0 = all)")
	asJSON := fs.Bool("json", false, "emit the aggregation as JSON instead of tables")
	_ = fs.Parse(args)

	root, err := storepkg.ResolveRoot(*storeRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppg report: %v\n", err)
		return 1
	}
	events := readJournal(root)
	if *since > 0 {
		cutoff := time.Now().UTC().Add(-*since)
		events = slices.DeleteFunc(events, func(e journal.Event) bool { return e.Time.Before(cutoff) })
	}
	if *sessionFilter != "" {
		events = slices.DeleteFunc(events, func(e journal.Event) bool {
			return !strings.HasPrefix(e.SessionID, *sessionFilter)
		})
	}
	if len(events) == 0 {
		fmt.Println("no events recorded (journal: " + filepath.Join(root, journal.FileName) + ")")
		return 0
	}

	data := aggregate(events)

	if *asJSON {
		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "ppg report: %v\n", err)
			return 1
		}
		fmt.Println(string(out))
		return 0
	}
	renderReport(os.Stdout, data)
	return 0
}

// readJournal loads every journal file under root — the live events.jsonl and
// any rotated events-*.jsonl — sorted so rotated (older) files come first.
// A malformed line is reported and skipped, like the escalation log reader.
func readJournal(root string) []journal.Event {
	rotated, _ := filepath.Glob(filepath.Join(root, "events-*.jsonl"))
	sort.Strings(rotated) // events-<unix-ts>: lexical order is time order
	paths := append(rotated, filepath.Join(root, journal.FileName))

	var events []journal.Event
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "ppg report: %v\n", err)
			}
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
		line := 0
		for sc.Scan() {
			line++
			text := strings.TrimSpace(sc.Text())
			if text == "" {
				continue
			}
			var e journal.Event
			if err := json.Unmarshal([]byte(text), &e); err != nil {
				fmt.Fprintf(os.Stderr, "ppg report: %s line %d unreadable: %v\n", path, line, err)
				continue
			}
			events = append(events, e)
		}
		if err := sc.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "ppg report: reading %s: %v\n", path, err)
		}
		_ = f.Close()
	}
	return events
}

// sessionReport is the per-session summary of the governed loop.
type sessionReport struct {
	SessionID    string    `json:"session_id"`
	First        time.Time `json:"first"`
	Last         time.Time `json:"last"`
	Turns        int       `json:"turns"` // ppg.plan.locked count
	Intents      int       `json:"intents"`
	PlanRejected int       `json:"plan_rejected"`
	MaxRejectRun int       `json:"max_reject_run"` // longest rejected streak before a lock
	Conflicts    int       `json:"conflicts"`
	Blocks       int       `json:"guard_blocks"`
	Allows       int       `json:"guard_allows"`
	Verifies     int       `json:"verifies"`
	Flags        []string  `json:"flags,omitempty"`
}

// policyCount ranks one policy id by how often it was violated, with the
// event names it was seen in.
type policyCount struct {
	PolicyID string   `json:"policy_id"`
	Count    int      `json:"count"`
	Events   []string `json:"events"`
}

// reportData is the full aggregation — the JSON shape of `ppg report -json`.
type reportData struct {
	EventCount     int             `json:"event_count"`
	Sessions       []sessionReport `json:"sessions"`
	BlocksByReason map[string]int  `json:"blocks_by_reason"`
	TopInvariants  []policyCount   `json:"top_invariants"`
	Substitutions  int             `json:"plan_substitutions"`
}

// aggregate folds the event stream into reportData. Events are sorted by
// time first: the journal interleaves five writers whose appends are
// serialized but whose clocks stamp before the flock.
func aggregate(events []journal.Event) reportData {
	slices.SortStableFunc(events, func(a, b journal.Event) int { return a.Time.Compare(b.Time) })

	type sessionAcc struct {
		sessionReport
		rejectRun int
		flags     map[string]bool
	}
	acc := map[string]*sessionAcc{}
	order := []string{}
	sessionOf := func(e journal.Event) *sessionAcc {
		sid := e.SessionID
		if sid == "" {
			sid = "(no session)"
		}
		s, ok := acc[sid]
		if !ok {
			s = &sessionAcc{sessionReport: sessionReport{SessionID: sid, First: e.Time}, flags: map[string]bool{}}
			acc[sid] = s
			order = append(order, sid)
		}
		s.Last = e.Time
		return s
	}

	blocksByReason := map[string]int{}
	invariants := map[string]*policyCount{}
	countPolicies := func(e journal.Event) {
		ids, _ := e.Attrs["policy_ids"].([]any)
		for _, raw := range ids {
			id, _ := raw.(string)
			if id == "" {
				continue
			}
			pc, ok := invariants[id]
			if !ok {
				pc = &policyCount{PolicyID: id}
				invariants[id] = pc
			}
			pc.Count++
			if !slices.Contains(pc.Events, e.Name) {
				pc.Events = append(pc.Events, e.Name)
			}
		}
	}
	substitutions := 0

	for _, e := range events {
		s := sessionOf(e)
		switch e.Name {
		case journal.EventIntentDeclared:
			s.Intents++
		case journal.EventPlanLocked:
			s.Turns++
			s.rejectRun = 0
			if s.Turns >= 2 {
				s.flags["re-locked"] = true
			}
		case journal.EventPlanRejected:
			s.PlanRejected++
			s.rejectRun++
			s.MaxRejectRun = max(s.MaxRejectRun, s.rejectRun)
			countPolicies(e)
		case journal.EventPlanConflict:
			s.Conflicts++
			s.flags["conflict"] = true
			countPolicies(e)
		case journal.EventPlanSubstitution:
			substitutions++
			s.flags["substitution"] = true
		case journal.EventGuardBlock:
			s.Blocks++
			reason, _ := e.Attrs["reason_code"].(string)
			blocksByReason[reason]++
			if reason == journal.ReasonSessionMismatch || reason == journal.ReasonTicketRejected {
				s.flags["bypass?"] = true
			}
		case journal.EventGuardAllow:
			s.Allows++
		case journal.EventArtifactRejected:
			countPolicies(e)
		case journal.EventChangesetOK:
			s.Verifies++
		case journal.EventChangesetRejected:
			s.Verifies++
			countPolicies(e)
		case journal.EventVerifyRun:
			s.Verifies++
			if outcome, _ := e.Attrs["outcome"].(string); outcome == "error" {
				s.flags["unverified-window"] = true
			}
		}
	}

	data := reportData{
		EventCount:     len(events),
		BlocksByReason: blocksByReason,
		Substitutions:  substitutions,
	}
	for _, sid := range order {
		s := acc[sid]
		if s.MaxRejectRun >= 2 {
			s.flags["asked-twice"] = true
		}
		for f := range s.flags {
			s.Flags = append(s.Flags, f)
		}
		sort.Strings(s.Flags)
		data.Sessions = append(data.Sessions, s.sessionReport)
	}
	for _, pc := range invariants {
		sort.Strings(pc.Events)
		data.TopInvariants = append(data.TopInvariants, *pc)
	}
	slices.SortFunc(data.TopInvariants, func(a, b policyCount) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.PolicyID, b.PolicyID)
	})
	return data
}

// renderReport prints the tabwriter tables (the escalations CLI style).
func renderReport(out *os.File, data reportData) {
	_, _ = fmt.Fprintf(out, "%d event(s)\n\nSESSIONS\n", data.EventCount)
	w := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SESSION\tFIRST\tTURNS\tINTENTS\tREJECTED\tMAX-REJECT-RUN\tBLOCKS\tALLOWS\tVERIFIES\tFLAGS")
	for _, s := range data.Sessions {
		_, _ = fmt.Fprintf(w, "%.8s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\n",
			s.SessionID, s.First.Format("2006-01-02 15:04"), s.Turns, s.Intents,
			s.PlanRejected, s.MaxRejectRun, s.Blocks, s.Allows, s.Verifies,
			strings.Join(s.Flags, ","))
	}
	_ = w.Flush()

	if len(data.BlocksByReason) > 0 {
		_, _ = fmt.Fprintln(out, "\nGUARD BLOCKS BY REASON")
		w = tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
		reasons := make([]string, 0, len(data.BlocksByReason))
		for r := range data.BlocksByReason {
			reasons = append(reasons, r)
		}
		sort.Strings(reasons)
		for _, r := range reasons {
			_, _ = fmt.Fprintf(w, "%s\t%d\n", r, data.BlocksByReason[r])
		}
		_ = w.Flush()
	}

	if len(data.TopInvariants) > 0 {
		_, _ = fmt.Fprintln(out, "\nTOP VIOLATED INVARIANTS")
		w = tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "POLICY\tCOUNT\tSEEN IN")
		for _, pc := range data.TopInvariants {
			_, _ = fmt.Fprintf(w, "%s\t%d\t%s\n", pc.PolicyID, pc.Count, strings.Join(pc.Events, ","))
		}
		_ = w.Flush()
	}

	if data.Substitutions > 0 {
		_, _ = fmt.Fprintf(out, "\n%d PLAN SUBSTITUTION(S) — a plan hash presented at verify time did not match the ticket.\n", data.Substitutions)
	}
}
