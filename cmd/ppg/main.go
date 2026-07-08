// Command ppg runs the Platform Planning Gateway PoC:
//
//	POST /enrich        — amplifier context (ADR invariants) for an intent
//	POST /lock_in_plan  — deterministic plan linter + capability ticket
//	POST /tools/{name}  — Smart Platform Tools (ticket verified in-tool)
//	GET  /debt_report   — transition-debt governance report
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/debt"
	"github.com/owulveryck/poc-agentic-platform/internal/enrich"
	"github.com/owulveryck/poc-agentic-platform/internal/linter"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/dbmigrate"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/patchcode"
	"github.com/owulveryck/poc-agentic-platform/internal/ticket"
)

func main() {
	addr := flag.String("addr", ":8000", "listen address")
	adrDir := flag.String("adr", "adr", "path to the ADR store")
	flag.Parse()

	store, err := adr.Load(*adrDir)
	if err != nil {
		log.Fatalf("loading ADR store: %v", err)
	}
	log.Printf("ADR store loaded: %d invariants", len(store.Invariants))

	lint, err := linter.New(store, *adrDir)
	if err != nil {
		log.Fatalf("loading plan linter: %v", err)
	}
	log.Printf("Plan linter ready: %d policies", len(lint.Registry))

	smarttools.Register(patchcode.Tool{}, "amplifier", "")
	smarttools.Register(dbmigrate.Tool{}, "amplifier", "")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /enrich", handleEnrich(store))
	mux.HandleFunc("POST /lock_in_plan", handleLockInPlan(lint))
	mux.HandleFunc("POST /tools/{name}", handleTool)
	mux.HandleFunc("GET /debt_report", handleDebtReport(lint.Registry))

	log.Printf("Platform Planning Gateway listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func handleEnrich(store *adr.Store) http.HandlerFunc {
	type request struct {
		Intent            string           `json:"intent"`
		RepositoryContext plan.RepoContext `json:"repository_context"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, enrich.Enrich(store, req.Intent, req.RepositoryContext))
	}
}

func handleLockInPlan(lint *linter.Linter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p plan.Plan
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := p.ValidateStructure(); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{
				"status": "PLAN_MALFORMED",
				"error":  err.Error(),
			})
			return
		}
		if violations := lint.Validate(&p); len(violations) > 0 {
			httpError(w, http.StatusUnprocessableEntity, map[string]any{
				"status":     "PLAN_REJECTED",
				"violations": violations,
				"guidance":   "Fix the violations above and resubmit the plan.",
			})
			return
		}
		tok, err := ticket.Issue(&p)
		if err != nil {
			httpError(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":           "PLAN_LOCKED",
			"plan_hash":        p.Hash(),
			"execution_ticket": tok,
		})
	}
}

func handleTool(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Ticket  string         `json:"ticket"`
		Targets []string       `json:"targets"`
		Payload map[string]any `json:"payload"`
	}
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	result, err := smarttools.Run(req.Ticket, r.PathValue("name"), req.Targets, req.Payload)
	if err != nil {
		var oos *smarttools.OutOfScopeError
		if errors.As(err, &oos) {
			httpError(w, http.StatusForbidden, map[string]any{
				"status":    "REFUSED",
				"code":      oos.Code,
				"attempted": oos.Attempted,
				"allowed":   oos.Allowed,
				"guidance":  "This action is not part of the locked plan. Re-plan through lock_in_plan if it is genuinely needed.",
			})
			return
		}
		httpError(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleDebtReport(registry map[string]linter.PolicyMeta) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, debt.Compute(registry))
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, v)
}
