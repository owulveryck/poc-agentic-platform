package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	storepkg "github.com/owulveryck/poc-agentic-platform/internal/store"
)

// runEscalations is the human half of the POLICY_CONFLICT loop: the server
// writes escalations (escalations.jsonl + conflicts.json under the state
// root); this CLI is what reads them, and `resolve` is how a conflict is
// closed after the corpus fix — so the capitalization loop has a consumer,
// not just a paper trail.
//
//	ppg escalations list [-all] [-store-root DIR]
//	ppg escalations show <conflict_id> [-store-root DIR]
//	ppg escalations resolve <conflict_id> [-note "…"] [-store-root DIR]
func runEscalations(args []string) int {
	if len(args) == 0 {
		escalationsUsage()
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return escalationsList(rest)
	case "show":
		return escalationsShow(rest)
	case "resolve":
		return escalationsResolve(rest)
	case "-h", "-help", "--help", "help":
		escalationsUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "ppg escalations: unknown subcommand %q\n", sub)
		escalationsUsage()
		return 1
	}
}

func escalationsUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  ppg escalations list    [-all] [-store-root DIR]     open conflicts (with -all: resolved too)
  ppg escalations show    <conflict_id> [-store-root DIR]   every recorded escalation for one conflict
  ppg escalations resolve <conflict_id> [-note "…"] [-store-root DIR]
        close a conflict after fixing the corpus; the running server adopts
        the resolution on reload (kill -HUP <ppg pid>)

The state root defaults to $PPG_STORE_ROOT, else $XDG_STATE_HOME/ppg.
`)
}

// escalationRecord is the shape of one escalations.jsonl line — either an
// escalation appended by the server or a resolution appended by this CLI
// (Type == "resolution"). Unknown fields are preserved in Raw for `show`.
type escalationRecord struct {
	TS         string   `json:"ts"`
	Type       string   `json:"type,omitempty"`
	ConflictID string   `json:"conflict_id"`
	SessionID  string   `json:"session_id,omitempty"`
	Intent     string   `json:"intent,omitempty"`
	SkillID    string   `json:"skill_id,omitempty"`
	Rejections int      `json:"rejections,omitempty"`
	PolicyIDs  []string `json:"policy_ids,omitempty"`
	Note       string   `json:"note,omitempty"`
	Raw        json.RawMessage
}

// idFirst splits "<conflict_id> [flags…]" so the id can come before the
// flags, as the usage string promises — stdlib flag parsing stops at the
// first positional argument, so "resolve <id> -note …" would otherwise
// silently ignore the flags. "resolve -note … <id>" keeps working too.
func idFirst(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func escalationsPaths(storeRoot string) (stateFile, logFile string, err error) {
	root, err := storepkg.ResolveRoot(storeRoot)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(root, "conflicts.json"), filepath.Join(root, "escalations.jsonl"), nil
}

// readEscalationLog parses escalations.jsonl. A missing file is an empty
// log; a malformed line is reported but does not hide the rest.
func readEscalationLog(path string) []escalationRecord {
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "ppg escalations: %v\n", err)
		}
		return nil
	}
	defer func() { _ = f.Close() }()
	var records []escalationRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var rec escalationRecord
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			fmt.Fprintf(os.Stderr, "ppg escalations: %s line %d unreadable: %v\n", path, line, err)
			continue
		}
		rec.Raw = json.RawMessage(text)
		records = append(records, rec)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "ppg escalations: reading %s: %v\n", path, err)
	}
	return records
}

func escalationsList(args []string) int {
	fs := flag.NewFlagSet("ppg escalations list", flag.ExitOnError)
	all := fs.Bool("all", false, "include resolved conflicts")
	storeRoot := fs.String("store-root", "", "per-machine state root (default $PPG_STORE_ROOT, else $XDG_STATE_HOME/ppg)")
	_ = fs.Parse(args)

	stateFile, logFile, err := escalationsPaths(*storeRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppg escalations: %v\n", err)
		return 1
	}
	detector := newConflictDetector(stateFile)
	records := readEscalationLog(logFile)

	resolutions := make(map[string]escalationRecord)
	for _, rec := range records {
		if rec.Type == "resolution" && rec.ConflictID != "" {
			resolutions[rec.ConflictID] = rec
		}
	}

	type row struct {
		id, status, first, policies, detail string
		rejections                          int
	}
	var rows []row
	for id, esc := range detector.state.Escalated {
		rows = append(rows, row{
			id:         id,
			status:     "OPEN",
			first:      esc.FirstTS,
			rejections: esc.Rejections,
			policies:   strings.Join(esc.PolicyIDs, ","),
			detail:     "session " + esc.SessionID,
		})
	}
	if *all {
		seen := make(map[string]bool, len(rows))
		for _, r := range rows {
			seen[r.id] = true
		}
		// Resolved conflicts only live in the log: latest escalation record
		// per id carries the facts, the resolution record carries the note.
		latest := make(map[string]escalationRecord)
		for _, rec := range records {
			if rec.Type == "" && rec.ConflictID != "" {
				latest[rec.ConflictID] = rec
			}
		}
		for id, res := range resolutions {
			if seen[id] {
				continue
			}
			esc := latest[id]
			rows = append(rows, row{
				id:         id,
				status:     "RESOLVED",
				first:      esc.TS,
				rejections: esc.Rejections,
				policies:   strings.Join(esc.PolicyIDs, ","),
				detail:     strings.TrimSpace("resolved " + res.TS + " " + res.Note),
			})
		}
	}
	if len(rows) == 0 {
		if *all {
			fmt.Println("no escalations recorded")
		} else {
			fmt.Println("no open conflicts")
		}
		return 0
	}
	slices.SortFunc(rows, func(a, b row) int { return strings.Compare(a.first, b.first) })
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "CONFLICT\tSTATUS\tFIRST SEEN\tREJECTIONS\tPOLICY IDS\tDETAIL")
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n", r.id, r.status, r.first, r.rejections, r.policies, r.detail)
	}
	_ = w.Flush()
	return 0
}

func escalationsShow(args []string) int {
	fs := flag.NewFlagSet("ppg escalations show", flag.ExitOnError)
	storeRoot := fs.String("store-root", "", "per-machine state root (default $PPG_STORE_ROOT, else $XDG_STATE_HOME/ppg)")
	id, rest := idFirst(args)
	_ = fs.Parse(rest)
	if id == "" && fs.NArg() == 1 {
		id = fs.Arg(0)
	}
	if id == "" || fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "ppg escalations show: exactly one <conflict_id> required")
		return 1
	}

	stateFile, logFile, err := escalationsPaths(*storeRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppg escalations: %v\n", err)
		return 1
	}
	detector := newConflictDetector(stateFile)
	if esc, ok := detector.state.Escalated[id]; ok {
		fmt.Printf("conflict %s: OPEN — policy set [%s], %d rejections since %s (first escalated by session %s)\n\n",
			id, strings.Join(esc.PolicyIDs, ", "), esc.Rejections, esc.FirstTS, esc.SessionID)
	} else {
		fmt.Printf("conflict %s: not currently open (resolved, or never escalated)\n\n", id)
	}

	found := false
	for _, rec := range readEscalationLog(logFile) {
		if rec.ConflictID != id {
			continue
		}
		found = true
		var pretty map[string]any
		if err := json.Unmarshal(rec.Raw, &pretty); err == nil {
			out, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Println(string(rec.Raw))
		}
	}
	if !found {
		fmt.Println("no log records for this conflict id")
	}
	return 0
}

func escalationsResolve(args []string) int {
	fs := flag.NewFlagSet("ppg escalations resolve", flag.ExitOnError)
	note := fs.String("note", "", "how the conflict was resolved (which policy/ADR was fixed) — recorded in the escalation log")
	storeRoot := fs.String("store-root", "", "per-machine state root (default $PPG_STORE_ROOT, else $XDG_STATE_HOME/ppg)")
	id, rest := idFirst(args)
	_ = fs.Parse(rest)
	if id == "" && fs.NArg() == 1 {
		id = fs.Arg(0)
	}
	if id == "" || fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "ppg escalations resolve: exactly one <conflict_id> required")
		return 1
	}

	stateFile, logFile, err := escalationsPaths(*storeRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppg escalations: %v\n", err)
		return 1
	}
	detector := newConflictDetector(stateFile)
	esc, ok := detector.resolve(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "ppg escalations resolve: no open conflict %q — `ppg escalations list` shows the open ids\n", id)
		return 1
	}
	appendEscalation(logFile, map[string]any{
		"ts":          time.Now().UTC().Format(time.RFC3339),
		"type":        "resolution",
		"conflict_id": id,
		"policy_ids":  esc.PolicyIDs,
		"note":        *note,
	})
	fmt.Printf("conflict %s resolved (policy set [%s]).\n", id, strings.Join(esc.PolicyIDs, ", "))
	fmt.Println("If the corpus fix is not already live, reload the running server so both land together: kill -HUP <ppg pid>.")
	fmt.Println("Until the reload, an in-memory copy of this escalation may still answer POLICY_CONFLICT.")
	return 0
}
