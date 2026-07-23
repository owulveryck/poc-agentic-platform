package journal

// Event names — the shared "ppg.*" vocabulary every emitter and every
// consumer (ppg report, the live dashboard, a future OTLP adapter) agrees
// on. One decision event is emitted exactly once, by the component that
// made the decision; clients emit only client-side facts the server cannot
// see (retries, ticket persistence, transport failures, local guard
// verdicts). See docs/reference/telemetry-events.md for the full contract
// (emitter, severity, attributes) of each event.
const (
	// Session lifecycle — emitted by the guards' SessionStart hook.
	EventSessionStart = "ppg.session.start"

	// Loop entry: a client declared an intent. Emitted by the MCP server
	// (and optionally preflight). The MCP server also forwards its active
	// session id inside the /enrich and /discover_service requests, so the
	// server-side events below carry the same session; this client event
	// remains the loop-entry marker even for callers that don't.
	EventIntentDeclared = "ppg.intent.declared"

	// Server-side pillar-1 events.
	EventEnrichServed      = "ppg.enrich.served"
	EventServiceDiscovered = "ppg.service.discovered"

	// Plan lifecycle — all emitted by the validation server, which owns
	// the lock decision.
	EventPlanMalformed    = "ppg.plan.malformed"
	EventPlanRejected     = "ppg.plan.rejected"
	EventPlanConflict     = "ppg.plan.conflict"
	EventPlanLocked       = "ppg.plan.locked"
	EventPlanSubstitution = "ppg.plan.substitution"

	// Skill registration (server).
	EventSkillRegistered = "ppg.skill.registered"
	EventSkillRejected   = "ppg.skill.rejected"

	// Content and scope decisions (server).
	EventArtifactRejected  = "ppg.artifact.rejected"
	EventScopeRefused      = "ppg.scope.refused"
	EventChangesetOK       = "ppg.changeset.ok"
	EventChangesetRejected = "ppg.changeset.rejected"

	// In-loop guard verdicts (guards). guard.allow is the "act" counter;
	// guard.block carries attribute reason_code, one of the Reason*
	// constants below.
	EventGuardAllow = "ppg.guard.allow"
	EventGuardBlock = "ppg.guard.block"

	// Client-side facts (MCP server).
	EventTicketSaved = "ppg.ticket.saved"
	EventLockRetry   = "ppg.lock.retry"
	EventClientError = "ppg.client.error"

	// Apply-time backstop outcome (ppg-verify).
	EventVerifyRun = "ppg.verify.run"
)

// Guard block reason codes — the machine-readable reason_code attribute of
// EventGuardBlock. ReasonGuardError marks a fail-closed infrastructure
// block (Severity ERROR); every other code is a policy denial (WARN).
const (
	ReasonNoTicket           = "no_ticket"
	ReasonOutOfPlanScope     = "out_of_plan_scope"
	ReasonSessionMismatch    = "session_mismatch"
	ReasonTicketRejected     = "ticket_rejected"
	ReasonInvariantViolation = "invariant_violation"
	ReasonGuardError         = "guard_error"
)
