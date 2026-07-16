// Command ppg runs the Platform Planning Gateway PoC:
//
//	POST /enrich           — amplifier context (ADR invariants) for an intent
//	POST /lock_in_plan     — deterministic plan linter + capability ticket
//	POST /tools/{name}     — Smart Platform Tools (ticket verified in-tool)
//	GET  /debt_report      — transition-debt governance report
//	POST /validate_skill   — enterprise skill governance linter (structure + security tier)
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/debt"
	"github.com/owulveryck/poc-agentic-platform/internal/enrich"
	"github.com/owulveryck/poc-agentic-platform/internal/linter"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/skill"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/dbmigrate"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/patchcode"
	"github.com/owulveryck/poc-agentic-platform/internal/ticket"
)

func main() {
	addr := flag.String("addr", ":8765", "listen address")
	adrDir := flag.String("adr", "adr", "path to the ADR store")
	skillGovDir := flag.String("skill-governance", "skill-governance", "path to the skill governance Rego policy directory")
	ticketTTLFlag := flag.Duration("ticket-ttl", 0,
		"capability ticket lifetime (0 = $PPG_TICKET_TTL, else the built-in default); the session still bounds it")
	flag.Parse()

	ttl, err := resolveTicketTTL(*ticketTTLFlag)
	if err != nil {
		log.Fatalf("resolving ticket TTL: %v", err)
	}

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

	skillLint, err := skill.NewLinter(*skillGovDir)
	if err != nil {
		log.Fatalf("loading skill governance linter: %v", err)
	}
	log.Printf("Skill governance linter ready")

	smarttools.Register(patchcode.Tool{}, "amplifier", "")
	smarttools.Register(dbmigrate.Tool{}, "amplifier", "")

	// Smart Tools enforce the artifact-view policy against the content they are
	// handed, reusing the same corpus as the plan linter and the guards.
	smarttools.SetArtifactEvaluator(func(path, content string) []string {
		var msgs []string
		for _, v := range lint.EvaluateArtifact(linter.Artifact{Path: path, Content: content}) {
			msgs = append(msgs, v.Message)
		}
		return msgs
	})

	mux := buildMux(store, lint, skillLint, ttl)

	log.Printf("Capability ticket TTL: %s (bounded by the session)", ttl)
	log.Printf("Platform Planning Gateway listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// buildMux wires the gateway routes. All handlers close over dependencies that
// are read-only after construction, so the returned mux is safe to serve
// concurrently (see cmd/ppg/main_test.go, which exercises it under -race).
func buildMux(store *adr.Store, lint *linter.Linter, skillLint *skill.Linter, ttl time.Duration) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /enrich", handleEnrich(store))
	mux.HandleFunc("POST /lock_in_plan", handleLockInPlan(lint, ttl))
	mux.HandleFunc("POST /tools/{name}", handleTool)
	mux.HandleFunc("POST /verify_artifact", handleVerifyArtifact(lint))
	mux.HandleFunc("POST /verify_changeset", handleVerifyChangeset(lint))
	mux.HandleFunc("GET /debt_report", handleDebtReport(lint.Registry))
	mux.HandleFunc("POST /validate_skill", handleValidateSkill(skillLint))
	return mux
}

// resolveTicketTTL picks the ticket lifetime: the -ticket-ttl flag when > 0,
// else $PPG_TICKET_TTL (a Go duration like "8h" or "30m"), else the built-in
// default. A malformed env value is a startup error rather than a silent
// fallback.
func resolveTicketTTL(flagValue time.Duration) (time.Duration, error) {
	if flagValue > 0 {
		return flagValue, nil
	}
	if v := os.Getenv("PPG_TICKET_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("invalid PPG_TICKET_TTL %q: %w", v, err)
		}
		if d <= 0 {
			return 0, fmt.Errorf("PPG_TICKET_TTL must be positive, got %q", v)
		}
		return d, nil
	}
	return ticket.DefaultTTL, nil
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

func handleLockInPlan(lint *linter.Linter, ttl time.Duration) http.HandlerFunc {
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
		tok, err := ticket.IssueWithTTL(&p, ttl)
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

// handleVerifyArtifact evaluates the policy corpus (artifact view) against one
// edited file's actual content — the in-loop check the guards and Smart Tools
// call. It verifies the ticket and path scope first, then the content policy.
func handleVerifyArtifact(lint *linter.Linter) http.HandlerFunc {
	type request struct {
		Ticket  string `json:"ticket"`
		Path    string `json:"path"`
		Content string `json:"content"`
		Op      string `json:"op"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if _, err := smarttools.GuardTargets(req.Ticket, []string{req.Path}); err != nil {
			writeGuardError(w, err)
			return
		}
		violations := lint.EvaluateArtifact(linter.Artifact{Path: req.Path, Content: req.Content, Op: req.Op})
		if len(violations) > 0 {
			httpError(w, http.StatusUnprocessableEntity, map[string]any{
				"status":     "ARTIFACT_REJECTED",
				"violations": violations,
				"guidance":   "The edited content violates an architectural invariant. Fix the content per the messages above; the file scope itself is allowed.",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ARTIFACT_OK"})
	}
}

// handleVerifyChangeset evaluates the corpus (changeset view) against a whole
// diff — the apply-time backstop. It verifies the ticket, that every changed
// path is in scope, and (when the caller supplies plan_hash) that the plan being
// executed still matches the one the ticket was issued for.
func handleVerifyChangeset(lint *linter.Linter) http.HandlerFunc {
	type request struct {
		Ticket   string            `json:"ticket"`
		Files    []linter.Artifact `json:"files"`
		PlanHash string            `json:"plan_hash"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		paths := make([]string, len(req.Files))
		for i, f := range req.Files {
			paths[i] = f.Path
		}
		claims, err := smarttools.GuardTargets(req.Ticket, paths)
		if err != nil {
			writeGuardError(w, err)
			return
		}
		if req.PlanHash != "" && req.PlanHash != claims.PlanHash {
			httpError(w, http.StatusConflict, map[string]any{
				"status":   "PLAN_SUBSTITUTION",
				"expected": claims.PlanHash,
				"got":      req.PlanHash,
				"guidance": "The plan being executed does not match the one this ticket was issued for. Re-plan through lock_in_plan.",
			})
			return
		}
		violations := lint.EvaluateChangeset(linter.Changeset{Files: req.Files, PlanHash: req.PlanHash})
		if len(violations) > 0 {
			httpError(w, http.StatusUnprocessableEntity, map[string]any{
				"status":     "CHANGESET_REJECTED",
				"violations": violations,
				"guidance":   "The changeset violates an architectural invariant. Fix the content per the messages above and re-verify.",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "CHANGESET_OK"})
	}
}

// writeGuardError renders a smarttools guard failure: a scope refusal as 403
// REFUSED, any other (invalid/expired ticket) as 401.
func writeGuardError(w http.ResponseWriter, err error) {
	var oos *smarttools.OutOfScopeError
	if errors.As(err, &oos) {
		httpError(w, http.StatusForbidden, map[string]any{
			"status":    "REFUSED",
			"code":      oos.Code,
			"attempted": oos.Attempted,
			"allowed":   oos.Allowed,
			"guidance":  "This target is not part of the locked plan's scope. Re-plan through lock_in_plan if it is genuinely needed.",
		})
		return
	}
	httpError(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
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
		writeGuardError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleValidateSkill(lint *skill.Linter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var s skill.Skill
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if violations := lint.Validate(&s); len(violations) > 0 {
			httpError(w, http.StatusUnprocessableEntity, map[string]any{
				"status":     "SKILL_REJECTED",
				"violations": violations,
				"guidance":   "Fix the violations above before publishing the skill to the registry.",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "SKILL_VALID",
			"tier":   lint.Tier(&s),
		})
	}
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
