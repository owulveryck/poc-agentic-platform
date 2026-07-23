// Command ppg runs the validation server PoC:
//
//	POST /enrich           — amplifier context (ADR invariants) for an intent
//	POST /lock_in_plan     — deterministic plan linter + capability ticket
//	POST /register_skill   — session-scoped SKILL.rego upload (client-pushed)
//	POST /tools/{name}     — Smart Platform Tools (ticket verified in-tool)
//	POST /verify_artifact  — in-loop content check (guard hook, Smart Tools)
//	POST /verify_changeset — apply-time content check (ppg-verify, CI)
//	POST /discover_service — policy-ranked service catalog: the sanctioned service for a capability
//	GET  /services         — list the service catalog
//	GET  /services/{id}    — one catalog record
//	GET  /debt_report      — transition-debt governance report
//	POST /validate_skill   — enterprise skill governance linter (structure + security tier)
//
// It also carries the escalation consumer CLI — the human half of the
// POLICY_CONFLICT loop:
//
//	ppg escalations list|show|resolve
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/catalog"
	"github.com/owulveryck/poc-agentic-platform/internal/debt"
	"github.com/owulveryck/poc-agentic-platform/internal/enrich"
	"github.com/owulveryck/poc-agentic-platform/internal/journal"
	"github.com/owulveryck/poc-agentic-platform/internal/linter"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/skill"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/dbmigrate"
	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/patchcode"
	storepkg "github.com/owulveryck/poc-agentic-platform/internal/store"
	"github.com/owulveryck/poc-agentic-platform/internal/ticket"
	"github.com/owulveryck/poc-agentic-platform/internal/version"
)

func main() {
	// Subcommand dispatch before flag.Parse: `ppg escalations …` is the
	// offline consumer of the conflict state, `ppg report` the offline
	// consumer of the decision-event journal — neither starts the server.
	if len(os.Args) > 1 && os.Args[1] == "escalations" {
		os.Exit(runEscalations(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "report" {
		os.Exit(runReport(os.Args[2:]))
	}

	addr := flag.String("addr", "127.0.0.1:8765",
		"listen address; defaults to loopback because the API is unauthenticated — pass an explicit host (e.g. :8765) only behind a trusted network or an auth proxy")
	adrDir := flag.String("adr", "", "path to the ADR store (optional; omit to run on skill companions and built-in rules only; demo corpus: examples/adr)")
	designTokens := flag.String("design-tokens", "design/tokens.css",
		"path to the canonical design tokens injected into the live dashboard (absent file: unstyled dashboard)")
	skillGovDir := flag.String("skill-governance", "skill-governance", "path to the skill governance Rego policy directory")
	skillsDir := flag.String("skills", "", "path to the published skills directory (one subdir per skill with SKILL.md [+ SKILL.rego]); enables Gate 3 for plans that declare skill_id")
	servicesDir := flag.String("services", "", "path to the service catalog directory (optional; omit to disable /discover_service)")
	servicePolicyDir := flag.String("service-policy", "", "path to the service-catalog ranking Rego policy directory (required with -services for /discover_service)")
	ticketTTLFlag := flag.Duration("ticket-ttl", 0,
		"capability ticket lifetime (0 = $PPG_TICKET_TTL, else the built-in default); the session still bounds it")
	allowWideScope := flag.Bool("allow-wide-scope", false,
		"accept plan targets like \".\" or \"*\" whose derived ticket would be allow-all (pre-1.0 behavior)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ppg " + version.String())
		return
	}

	ttl, err := resolveTicketTTL(*ticketTTLFlag)
	if err != nil {
		log.Fatalf("resolving ticket TTL: %v", err)
	}

	cfg := corpusConfig{
		adrDir:           *adrDir,
		skillsDir:        *skillsDir,
		skillGovDir:      *skillGovDir,
		servicesDir:      *servicesDir,
		servicePolicyDir: *servicePolicyDir,
		allowWideScope:   *allowWideScope,
	}
	c, err := loadCorpus(cfg)
	if err != nil {
		log.Fatalf("ppg: %v", err)
	}

	stateRoot, err := storepkg.ResolveRoot("")
	if err != nil {
		log.Fatalf("resolving per-machine state root: %v", err)
	}
	// escalationLog is the POLICY_CONFLICT paper trail (JSONL, append-only):
	// every livelock escalation lands here for the humans who own the rules.
	escalationLog := filepath.Join(stateRoot, "escalations.jsonl")

	// jw is the decision-event journal (see internal/journal): one wide JSONL
	// event per governance decision, shared with the guards and ppg-verify.
	// Opened once for the process lifetime — it only holds a path, so it is
	// safe to capture across SIGHUP corpus reloads. nil (telemetry disabled)
	// is a valid, inert writer.
	jw := journal.Open(stateRoot, "ppg", "")

	// The conflict detector is created ONCE for the process lifetime, not per
	// mux: buildMux runs again on every SIGHUP reload, so a detector owned by
	// the mux would have its livelock counters silently wiped on each reload —
	// and a reload is exactly how a human applies the corpus fix, which would
	// reset the counter just as the conflict is being resolved. Threading one
	// detector through install keeps counter state stable across reloads, and
	// the state file keeps it stable across restarts.
	conflicts := newConflictDetector(filepath.Join(stateRoot, "conflicts.json"))

	// The ticket signing key is never hardcoded: $PPG_TICKET_SECRET wins,
	// else a per-machine key is generated once under the state root.
	if os.Getenv(ticket.EnvSecret) == "" {
		keyFile := filepath.Join(stateRoot, "ticket.key")
		if err := ticket.UseKeyFile(keyFile); err != nil {
			log.Fatalf("loading ticket signing key: %v", err)
		}
		log.Printf("Ticket signing key: %s", keyFile)
	} else {
		log.Printf("Ticket signing key: $%s", ticket.EnvSecret)
	}

	smarttools.Register(patchcode.Tool{}, "amplifier", "")
	smarttools.Register(dbmigrate.Tool{}, "amplifier", "")

	// install wires one loaded corpus into everything that consumes it: the
	// Smart Tools' artifact evaluator and the HTTP routes. It is called at
	// startup and again on every successful SIGHUP reload — the routes swap
	// atomically via the reloadableHandler.
	handler := &reloadableHandler{}
	install := func(c *corpus) {
		// Smart Tools enforce the artifact-view policy against the content
		// they are handed, reusing the same corpus as the plan linter and the
		// guards. When the ticket carries a skill_id, that skill's companion
		// Rego joins the ADR corpus so per-edit skill invariants also fire —
		// the session_id selects between operator-provided skills and
		// client-uploaded, session-scoped ones.
		lint := c.lint
		smarttools.SetArtifactEvaluator(func(path, content, skillID, sessionID string) []string {
			var msgs []string
			for _, v := range lint.EvaluateArtifact(sessionID, skillID, linter.Artifact{Path: path, Content: content}) {
				msgs = append(msgs, v.Message)
			}
			return msgs
		})
		handler.mux.Store(buildMux(c.store, c.lint, c.skillLint, c.catStore, c.ranker, ttl, conflicts, escalationLog, jw))
	}
	install(c)

	// Hot reload: SIGHUP rebuilds the whole corpus from disk — capitalizing
	// a new or extended policy no longer requires a restart. Fail-safe: a
	// reload error keeps the previous corpus serving. Session-scoped skill
	// registrations survive the swap (AdoptSessions).
	go func() {
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		for range hup {
			nc, err := loadCorpus(cfg)
			if err != nil {
				log.Printf("SIGHUP reload failed — keeping the previous corpus: %v", err)
				continue
			}
			nc.lint.AdoptSessions(c.lint)
			c = nc
			install(nc)
			// Adopt conflict resolutions recorded by `ppg escalations
			// resolve` — resolving a conflict rides the same SIGHUP ritual
			// as capitalizing the corpus fix. Live rejection counters are
			// kept.
			conflicts.syncFromDisk()
			log.Printf("SIGHUP: corpus reloaded")
		}
	}()

	// The live-observation routes are mounted OUTSIDE the reloadable corpus
	// mux: they depend only on the immutable journal path, so a SIGHUP corpus
	// reload never interrupts a running event stream.
	root := http.NewServeMux()
	root.HandleFunc("GET /events", servePage(*designTokens, dashboardHTML))
	root.HandleFunc("GET /events/loop", servePage(*designTokens, loopHTML))
	root.HandleFunc("GET /events/stream", handleEventStream(filepath.Join(stateRoot, journal.FileName), streamPollInterval))
	root.Handle("/", handler)

	log.Printf("Capability ticket TTL: %s (bounded by the session)", ttl)
	log.Printf("Live dashboard: http://%s/events", *addr)
	log.Printf("validation server listening on %s", *addr)
	// The validation server accepts untrusted POSTed plans/artifacts: bound both the
	// request body size and the connection lifetimes.
	srv := &http.Server{
		Addr:              *addr,
		Handler:           http.MaxBytesHandler(root, maxRequestBody),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Minute,
		WriteTimeout:      time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	log.Fatal(srv.ListenAndServe())
}

// reloadableHandler serves the current mux behind an atomic pointer so a
// SIGHUP corpus reload swaps every route's dependencies in one step, with no
// locking on the request path.
type reloadableHandler struct{ mux atomic.Pointer[http.ServeMux] }

func (h *reloadableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.Load().ServeHTTP(w, r)
}

// maxRequestBody caps any request body: /verify_changeset carries the full
// content of every changed file, so the cap is generous but finite.
const maxRequestBody = 16 << 20 // 16 MiB

// buildMux wires the validation server routes. All handlers close over dependencies that
// are read-only after construction — except the conflict detector, which is
// internally synchronized — so the returned mux is safe to serve
// concurrently (see cmd/ppg/main_test.go, which exercises it under -race).
func buildMux(store *adr.Store, lint *linter.Linter, skillLint *skill.Linter, catStore *catalog.Store, ranker *catalog.Ranker, ttl time.Duration, conflicts *conflictDetector, escalationLog string, jw *journal.Writer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /enrich", handleEnrich(store, jw))
	mux.HandleFunc("POST /lock_in_plan", handleLockInPlan(lint, ttl, conflicts, escalationLog, jw))
	mux.HandleFunc("POST /register_skill", handleRegisterSkill(lint, jw))
	mux.HandleFunc("POST /tools/{name}", handleTool(jw))
	mux.HandleFunc("POST /verify_artifact", handleVerifyArtifact(lint, jw))
	mux.HandleFunc("POST /verify_changeset", handleVerifyChangeset(lint, jw))
	mux.HandleFunc("POST /discover_service", handleDiscoverService(catStore, ranker, jw))
	mux.HandleFunc("GET /services", handleListServices(catStore))
	mux.HandleFunc("GET /services/{id}", handleGetService(catStore))
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

func handleEnrich(store *adr.Store, jw *journal.Writer) http.HandlerFunc {
	type request struct {
		Intent            string           `json:"intent"`
		SessionID         string           `json:"session_id"`
		RepositoryContext plan.RepoContext `json:"repository_context"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.Intent) == "" {
			httpError(w, http.StatusBadRequest, map[string]any{"error": "intent is required"})
			return
		}
		out := enrich.Enrich(store, req.Intent, req.RepositoryContext)
		// session_id is optional in the request (the MCP server sends its
		// active session); when present, the event is attributed to that
		// session — the per-session loop view shows the exchange.
		jw.Emit(journal.Event{
			Name:      journal.EventEnrichServed,
			SessionID: req.SessionID,
			Attrs: map[string]any{
				"intent":          req.Intent,
				"repo":            req.RepositoryContext.Name,
				"invariant_count": len(out.AmplifierContext.ArchitecturalInvariants),
			},
		})
		writeJSON(w, http.StatusOK, out)
	}
}

func handleLockInPlan(lint *linter.Linter, ttl time.Duration, conflicts *conflictDetector, escalationLog string, jw *journal.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p plan.Plan
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := p.ValidateStructure(); err != nil {
			resp := map[string]any{
				"status": "PLAN_MALFORMED",
				"error":  err.Error(),
			}
			attrs := map[string]any{"reason": err.Error()}
			attachPayload(attrs, "request", p)
			attachPayload(attrs, "response", resp)
			jw.Emit(journal.Event{
				Name:      journal.EventPlanMalformed,
				Severity:  journal.SeverityWarn,
				SessionID: p.SessionID,
				Attrs:     attrs,
			})
			httpError(w, http.StatusBadRequest, resp)
			return
		}
		if violations := lint.Validate(&p); len(violations) > 0 {
			ids := violationPolicyIDs(violations)
			ts := time.Now().UTC().Format(time.RFC3339)
			rejections, cid, blocked := conflicts.observeRejection(p.SessionID, ids, ts)
			if blocked {
				// Livelock: this exact violation set has been rejected
				// conflictThreshold times without a successful lock in
				// between (or was already escalated — by any session).
				// "Fix and resubmit" is no longer honest guidance —
				// escalate to the humans who own the clashing rules, and
				// keep blocking until `ppg escalations resolve`.
				sources := policySources(lint, ids)
				appendEscalation(escalationLog, map[string]any{
					"ts":             ts,
					"conflict_id":    cid,
					"session_id":     p.SessionID,
					"intent":         p.Intent,
					"skill_id":       p.SkillID,
					"plan_steps":     p.Steps,
					"rejections":     rejections,
					"policy_ids":     ids,
					"policy_sources": sources,
					"violations":     violations,
				})
				log.Printf("POLICY_CONFLICT %s: violation set %v rejected %d times (session %s) — escalation recorded in %s",
					cid, ids, rejections, p.SessionID, escalationLog)
				resp := map[string]any{
					"status":         "POLICY_CONFLICT",
					"conflict_id":    cid,
					"violations":     violations,
					"policy_ids":     ids,
					"policy_sources": sources,
					"rejections":     rejections,
					"escalation_log": escalationLog,
					"guidance": "STOP resubmitting. Plans were rejected with this exact violation set " + strconv.Itoa(conflictThreshold) +
						"+ times without a successful lock in between: either these policies are mutually unsatisfiable for this " +
						"intent, or the required plan shape is not reachable from the current approach. This is now a human " +
						"decision — review the policies in policy_ids with their owners (policy_sources says whether each comes " +
						"from the ADR corpus, a skill companion, or a built-in rule), or change the intent. The escalation was " +
						"recorded; a human inspects it with `ppg escalations list` / `ppg escalations show " + cid + "`, fixes " +
						"the corpus, then runs `ppg escalations resolve " + cid + "` and reloads the server (SIGHUP). Until " +
						"then the validation server keeps answering POLICY_CONFLICT for this violation set, from every session.",
				}
				attrs := map[string]any{
					"conflict_id": cid,
					"policy_ids":  ids,
					"rejections":  rejections,
					"intent":      p.Intent,
					"skill_id":    p.SkillID,
				}
				attachPayload(attrs, "request", p)
				attachPayload(attrs, "response", resp)
				jw.Emit(journal.Event{
					Name:      journal.EventPlanConflict,
					Severity:  journal.SeverityWarn,
					SessionID: p.SessionID,
					Attrs:     attrs,
				})
				httpError(w, http.StatusConflict, resp)
				return
			}
			resp := map[string]any{
				"status":     "PLAN_REJECTED",
				"violations": violations,
				"guidance":   "Fix the violations above and resubmit the plan.",
			}
			attrs := map[string]any{
				"policy_ids":      ids,
				"violation_count": len(violations),
				"rejection_count": rejections,
				"intent":          p.Intent,
				"skill_id":        p.SkillID,
			}
			attachPayload(attrs, "request", p)
			attachPayload(attrs, "response", resp)
			jw.Emit(journal.Event{
				Name:      journal.EventPlanRejected,
				Severity:  journal.SeverityWarn,
				SessionID: p.SessionID,
				Attrs:     attrs,
			})
			httpError(w, http.StatusUnprocessableEntity, resp)
			return
		}
		conflicts.observeSuccess(p.SessionID)
		planHash, err := p.Hash()
		if err != nil {
			httpError(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tok, err := ticket.IssueWithTTL(&p, ttl)
		if err != nil {
			httpError(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		targets := 0
		for _, s := range p.Steps {
			targets += len(s.Targets)
		}
		attrs := map[string]any{
			"plan_hash":    planHash,
			"step_count":   len(p.Steps),
			"target_count": targets,
			"intent":       p.Intent,
			"skill_id":     p.SkillID,
			"ticket_ttl_s": int(ttl.Seconds()),
		}
		attachPayload(attrs, "request", p)
		// The journaled response deliberately excludes the execution ticket:
		// a bearer credential never belongs in telemetry.
		attachPayload(attrs, "response", map[string]any{
			"status":    "PLAN_LOCKED",
			"plan_hash": planHash,
		})
		jw.Emit(journal.Event{
			Name:      journal.EventPlanLocked,
			SessionID: p.SessionID,
			Attrs:     attrs,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"status":           "PLAN_LOCKED",
			"plan_hash":        planHash,
			"execution_ticket": tok,
		})
	}
}

// handleVerifyArtifact evaluates the policy corpus (artifact view) against one
// edited file's actual content — the in-loop check the guards and Smart Tools
// call. It verifies the ticket and path scope first, then the content policy.
func handleVerifyArtifact(lint *linter.Linter, jw *journal.Writer) http.HandlerFunc {
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
		if strings.TrimSpace(req.Path) == "" {
			httpError(w, http.StatusBadRequest, map[string]any{"error": "path is required"})
			return
		}
		claims, err := smarttools.GuardTargets(req.Ticket, []string{req.Path})
		if err != nil {
			writeGuardError(w, err, jw, "")
			return
		}
		violations := lint.EvaluateArtifact(claims.SessionID, claims.SkillID, linter.Artifact{Path: req.Path, Content: req.Content, Op: req.Op})
		if len(violations) > 0 {
			// Only rejections are journaled: an ARTIFACT_OK is already visible
			// as the guard's ppg.guard.allow, and would double per-edit volume.
			// The request payload is path+op only — the edited CONTENT never
			// enters the journal (privacy contract).
			resp := map[string]any{
				"status":     "ARTIFACT_REJECTED",
				"violations": violations,
				"guidance":   "The edited content violates an architectural invariant. Fix the content per the messages above; the file scope itself is allowed.",
			}
			attrs := map[string]any{
				"path":       req.Path,
				"op":         req.Op,
				"policy_ids": violationPolicyIDs(violations),
			}
			attachPayload(attrs, "response", resp)
			jw.Emit(journal.Event{
				Name:      journal.EventArtifactRejected,
				Severity:  journal.SeverityWarn,
				SessionID: claims.SessionID,
				Attrs:     attrs,
			})
			httpError(w, http.StatusUnprocessableEntity, resp)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ARTIFACT_OK"})
	}
}

// handleVerifyChangeset evaluates the corpus (changeset view) against a whole
// diff — the apply-time backstop. It verifies the ticket, that every changed
// path is in scope, and (when the caller supplies plan_hash) that the plan being
// executed still matches the one the ticket was issued for.
func handleVerifyChangeset(lint *linter.Linter, jw *journal.Writer) http.HandlerFunc {
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
			writeGuardError(w, err, jw, "")
			return
		}
		if req.PlanHash != "" && req.PlanHash != claims.PlanHash {
			resp := map[string]any{
				"status":   "PLAN_SUBSTITUTION",
				"expected": claims.PlanHash,
				"got":      req.PlanHash,
				"guidance": "The plan being executed does not match the one this ticket was issued for. Re-plan through lock_in_plan.",
			}
			attrs := map[string]any{
				"expected_hash": claims.PlanHash,
				"got_hash":      req.PlanHash,
			}
			attachPayload(attrs, "response", resp)
			jw.Emit(journal.Event{
				Name:      journal.EventPlanSubstitution,
				Severity:  journal.SeverityWarn,
				SessionID: claims.SessionID,
				Attrs:     attrs,
			})
			httpError(w, http.StatusConflict, resp)
			return
		}
		violations := lint.EvaluateChangeset(claims.SessionID, claims.SkillID, linter.Changeset{Files: req.Files, PlanHash: req.PlanHash})
		if len(violations) > 0 {
			// Request payload = the changed paths, never their contents.
			resp := map[string]any{
				"status":     "CHANGESET_REJECTED",
				"violations": violations,
				"guidance":   "The changeset violates an architectural invariant. Fix the content per the messages above and re-verify.",
			}
			attrs := map[string]any{
				"file_count": len(req.Files),
				"policy_ids": violationPolicyIDs(violations),
			}
			attachPayload(attrs, "request", map[string]any{"paths": paths, "plan_hash": req.PlanHash})
			attachPayload(attrs, "response", resp)
			jw.Emit(journal.Event{
				Name:      journal.EventChangesetRejected,
				Severity:  journal.SeverityWarn,
				SessionID: claims.SessionID,
				Attrs:     attrs,
			})
			httpError(w, http.StatusUnprocessableEntity, resp)
			return
		}
		jw.Emit(journal.Event{
			Name:      journal.EventChangesetOK,
			SessionID: claims.SessionID,
			Attrs:     map[string]any{"file_count": len(req.Files)},
		})
		writeJSON(w, http.StatusOK, map[string]any{"status": "CHANGESET_OK"})
	}
}

// writeGuardError renders a smarttools guard failure: a scope refusal as 403
// REFUSED, any other (invalid/expired ticket) as 401. sessionID may be empty
// when the ticket itself could not be verified.
func writeGuardError(w http.ResponseWriter, err error, jw *journal.Writer, sessionID string) {
	var oos *smarttools.OutOfScopeError
	if errors.As(err, &oos) {
		resp := map[string]any{
			"status":    "REFUSED",
			"code":      oos.Code,
			"attempted": oos.Attempted,
			"allowed":   oos.Allowed,
			"guidance":  "This target is not part of the locked plan's scope. Re-plan through lock_in_plan if it is genuinely needed.",
		}
		attrs := map[string]any{
			"code":      oos.Code,
			"attempted": oos.Attempted,
		}
		attachPayload(attrs, "response", resp)
		jw.Emit(journal.Event{
			Name:      journal.EventScopeRefused,
			Severity:  journal.SeverityWarn,
			SessionID: sessionID,
			Attrs:     attrs,
		})
		httpError(w, http.StatusForbidden, resp)
		return
	}
	httpError(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
}

func handleTool(jw *journal.Writer) http.HandlerFunc {
	type request struct {
		Ticket  string         `json:"ticket"`
		Targets []string       `json:"targets"`
		Payload map[string]any `json:"payload"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		result, err := smarttools.Run(req.Ticket, r.PathValue("name"), req.Targets, req.Payload)
		if err != nil {
			writeGuardError(w, err, jw, "")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// handleRegisterSkill compiles a client-uploaded SKILL.rego and stores it in
// the linter's session-scoped tier. It is how a skill installed locally
// (e.g. via `apm install ... --target claude`) reaches a validation server that does
// not share the client's filesystem: the MCP server POSTs it here before
// forwarding lock_in_plan. Idempotent — re-uploading identical content is
// a no-op (the linter simply overwrites the same key with the same evaluator).
func handleRegisterSkill(lint *linter.Linter, jw *journal.Writer) http.HandlerFunc {
	type request struct {
		SessionID string `json:"session_id"`
		Name      string `json:"name"`
		SkillMD   string `json:"skill_md"`
		SkillRego string `json:"skill_rego"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.SessionID) == "" {
			httpError(w, http.StatusBadRequest, map[string]any{"error": "session_id is required"})
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			httpError(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
			return
		}
		if err := lint.RegisterSessionSkill(req.SessionID, req.Name, req.SkillRego); err != nil {
			jw.Emit(journal.Event{
				Name:      journal.EventSkillRejected,
				Severity:  journal.SeverityWarn,
				SessionID: req.SessionID,
				Attrs:     map[string]any{"skill": req.Name},
			})
			httpError(w, http.StatusUnprocessableEntity, map[string]any{
				"status":   "SKILL_COMPILE_ERROR",
				"error":    err.Error(),
				"guidance": "Fix the Rego source and re-register the skill; the previous registration under this name (if any) still applies.",
			})
			return
		}
		jw.Emit(journal.Event{
			Name:      journal.EventSkillRegistered,
			SessionID: req.SessionID,
			Attrs:     map[string]any{"skill": req.Name, "has_rego": req.SkillRego != ""},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "SKILL_REGISTERED",
			"session_id": req.SessionID,
			"name":       req.Name,
			"has_rego":   req.SkillRego != "",
		})
	}
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

// discoveredService is the shape returned for a catalog entry. The recommended
// service carries the full endpoint + API usage; alternatives are lighter.
type discoveredService struct {
	ServiceID    string `json:"service_id"`
	Name         string `json:"name"`
	Capability   string `json:"capability,omitempty"`
	Status       string `json:"status"`
	Tier         int    `json:"tier,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	OwnerTeam    string `json:"owner_team,omitempty"`
	APIUsage     string `json:"api_usage,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// handleDiscoverService answers "which sanctioned service should I use for this
// capability?" — the discovery counterpart of /enrich. It retrieves the
// candidates for a capability (or intent), ranks them with the policy-as-code
// ranker, and returns the recommended service (with endpoint + API usage) plus
// the alternatives and why each was or was not chosen.
func handleDiscoverService(catStore *catalog.Store, ranker *catalog.Ranker, jw *journal.Writer) http.HandlerFunc {
	type request struct {
		Capability        string         `json:"capability"`
		Intent            string         `json:"intent"`
		SessionID         string         `json:"session_id"`
		RepositoryContext map[string]any `json:"repository_context"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if catStore == nil || ranker == nil {
			httpError(w, http.StatusServiceUnavailable, map[string]any{
				"status":   "SERVICE_CATALOG_UNAVAILABLE",
				"guidance": "The validation server was started without a service catalog and/or ranking policy (see -services / -service-policy).",
			})
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.Capability) == "" && strings.TrimSpace(req.Intent) == "" {
			httpError(w, http.StatusBadRequest, map[string]any{"error": "capability or intent is required"})
			return
		}
		candidates := catStore.Retrieve(req.Capability, req.Intent)
		capability := req.Capability
		if capability == "" && len(candidates) > 0 {
			capability = candidates[0].Capability
		}
		if len(candidates) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":       "NO_SERVICE_FOR_CAPABILITY",
				"capability":   capability,
				"alternatives": []discoveredService{},
				"policy_notes": []string{},
			})
			return
		}
		ranked, err := ranker.Rank(capability, req.RepositoryContext, candidates)
		if err != nil {
			httpError(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		var recommended *discoveredService
		alternatives := []discoveredService{}
		policyNotes := []string{}
		for _, rk := range ranked {
			ds := discoveredService{
				ServiceID:    rk.Service.ServiceID,
				Name:         rk.Service.Name,
				Capability:   rk.Service.Capability,
				Status:       rk.Service.Status,
				Tier:         rk.Service.Tier,
				Endpoint:     rk.Service.Endpoint,
				OwnerTeam:    rk.Service.OwnerTeam,
				SupersededBy: rk.Service.SupersededBy,
				Reason:       rk.Verdict.Reason,
			}
			if recommended == nil && rk.Verdict.Allow {
				ds.APIUsage = rk.Service.APIUsage
				picked := ds
				recommended = &picked
				continue
			}
			alternatives = append(alternatives, ds)
			if !rk.Verdict.Allow && rk.Verdict.Reason != "" {
				policyNotes = append(policyNotes, rk.Verdict.Reason)
			}
		}

		status := "SERVICE_FOUND"
		if recommended == nil {
			status = "NO_SERVICE_FOR_CAPABILITY"
		}
		ev := journal.Event{
			Name:      journal.EventServiceDiscovered,
			SessionID: req.SessionID,
			Attrs: map[string]any{
				"capability":         capability,
				"status":             status,
				"alternatives_count": len(alternatives),
			},
		}
		if recommended != nil {
			ev.Attrs["service_id"] = recommended.ServiceID
		} else {
			ev.Severity = journal.SeverityWarn
		}
		jw.Emit(ev)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       status,
			"capability":   capability,
			"recommended":  recommended,
			"alternatives": alternatives,
			"policy_notes": policyNotes,
		})
	}
}

// handleListServices returns the whole catalog (metadata + API usage).
func handleListServices(catStore *catalog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if catStore == nil {
			writeJSON(w, http.StatusOK, map[string]any{"services": []catalog.Service{}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"services": catStore.All()})
	}
}

// handleGetService returns a single catalog record by service_id.
func handleGetService(catStore *catalog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if catStore != nil {
			if svc, ok := catStore.Get(id); ok {
				writeJSON(w, http.StatusOK, svc)
				return
			}
		}
		httpError(w, http.StatusNotFound, map[string]any{
			"status": "SERVICE_NOT_FOUND", "service_id": id,
		})
	}
}

// payloadCap bounds one captured request/response payload inside an event.
const payloadCap = 32 << 10 // 32 KiB

// attachPayload adds a bounded JSON payload attribute to a decision event's
// Attrs, honoring the PPG_TELEMETRY_PAYLOADS kill switch (see the journal
// privacy contract). Oversized payloads are replaced by a size note so an
// event stays a cheap wide row; the execution ticket and file contents are
// never passed here by construction.
func attachPayload(attrs map[string]any, key string, v any) {
	if journal.PayloadsDisabled() {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	if len(b) > payloadCap {
		attrs[key+"_omitted"] = fmt.Sprintf("%d bytes > %d cap", len(b), payloadCap)
		return
	}
	attrs[key] = json.RawMessage(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, v)
}
