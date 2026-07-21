// Package linter is the deterministic plan validator behind lock_in_plan.
//
// It is deliberately NOT an LLM: it evaluates Open Policy Agent (OPA/Rego)
// policies loaded from ADR-paired .rego files, so a non-conforming plan is
// rejected 100% of the time, reproducibly. Each policy is tagged with its
// nature on the durability axis (amplifier vs compensatory) and, when
// compensatory, carries a measurable sunset condition.
//
// Each ADR is a dual-representation governance artifact: the semantic
// directive (InvariantText) is injected at enrich() time to shape planning;
// the paired .rego file is evaluated at lock_in_plan time for deterministic
// enforcement. The two representations can have different lifetimes on the
// durability axis.
package linter

import (
	"fmt"
	"log"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/owulveryck/poc-agentic-platform/internal/adr"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/policy"
)

// Nature positions an artifact on the durability axis.
type Nature string

const (
	// Amplifier marks an artifact as a durable asset: its value increases as
	// model capabilities improve, so it is never scheduled for removal.
	Amplifier Nature = "amplifier"
	// Compensatory marks an artifact as temporary scaffolding: it compensates
	// for a current model limitation and must carry a measurable sunset
	// condition that determines when it can be removed.
	Compensatory Nature = "compensatory"
)

// PolicyMeta is the governance record of one policy.
type PolicyMeta struct {
	// Nature positions the policy on the durability axis (Amplifier or Compensatory).
	Nature Nature `json:"nature"`
	// Rationale explains why this policy exists and what invariant it enforces.
	Rationale string `json:"rationale"`
	// SunsetCondition is the measurable condition under which a Compensatory
	// policy can be removed. Empty for Amplifier policies.
	SunsetCondition string `json:"sunset_condition,omitempty"`
}

// Violation is a semantic, actionable rejection reason returned to the agent.
type Violation struct {
	// PolicyID identifies which policy was violated (matches a key in Registry).
	PolicyID string `json:"policy_id"`
	// Message is a human-readable, agent-facing explanation of the violation
	// and what must be changed for the plan to pass.
	Message string `json:"message"`
	// Nature mirrors the nature of the violated policy so consumers can
	// distinguish durable invariants from compensatory scaffolding rejections.
	Nature Nature `json:"nature"`
}

// Linter evaluates OPA/Rego policies derived from ADR .rego files. The same
// compiled corpus is evaluated at three altitudes — plan, artifact and
// changeset — discriminated by the input.view field (see Validate,
// EvaluateArtifact, EvaluateChangeset). When the caller supplies a skillID,
// the artifact and changeset evaluators additionally run that skill's
// companion Rego, so a SKILL.rego can enforce per-edit invariants alongside
// the ADR corpus (fail-closed on an unknown skill).
//
// Skill companions live in two tiers:
//
//   - **Operator-provided** — loaded from -skills at startup
//     (LoadSkillCompanions). Enterprise baseline; shared across sessions.
//   - **Session-scoped** — uploaded via RegisterSessionSkill (client push
//     over POST /register_skill). Isolated per session_id, evicted at
//     session teardown.
//
// The operator tier wins on name collision so a project-local upload cannot
// silently downgrade a policy the operator has already reserved.
type Linter struct {
	// Registry is the governance catalog of all tracked policies, keyed by
	// policy_id. Used by the debt report to measure the compensatory ratio.
	Registry map[string]PolicyMeta
	// AllowWideScope disables the built-in scope-breadth cap: when false (the
	// default), a plan step targeting the repository root ("." / "/" / "*")
	// is rejected at lock time, because the derived capability ticket would
	// be allow-all and least privilege would be meaningless.
	AllowWideScope bool
	eval           *policy.Evaluator
	// skillMu guards skillCompanions and sessionSkills for the lifetime of
	// the process. Reads are hot (every /verify_artifact and /lock_in_plan
	// with a skill id); writes only happen at startup and at
	// /register_skill (rare).
	skillMu sync.RWMutex
	// skillCompanions maps a published skill name to the compiled evaluator
	// of its companion Rego (Gate 3). A nil value marks a skill published
	// without a companion (tier 0). Populated by LoadSkillCompanions.
	skillCompanions map[string]*policy.Evaluator
	// sessionSkills is the session-scoped tier: (session_id → skill_name →
	// evaluator). Populated by RegisterSessionSkill; the operator tier
	// (skillCompanions) is consulted first, so a session upload cannot
	// override an operator-provided policy under the same name.
	sessionSkills map[string]map[string]*policy.Evaluator
}

// Artifact is one edited file's actual content — the artifact view of the
// policy input, used by the in-loop guard hook and the Smart Tools.
type Artifact struct {
	// Path is the file path being written, relative to the project root.
	Path string `json:"path"`
	// Content is the full proposed content of the file after the edit.
	Content string `json:"content"`
	// Op is the operation ("write", "edit", "create"); optional.
	Op string `json:"op,omitempty"`
}

// Changeset is a set of edited files — the changeset (diff) view of the policy
// input, used by the apply-time gate. PlanHash lets the gate detect plan
// substitution against the ticket.
type Changeset struct {
	// Files are the changed files with their post-change content.
	Files []Artifact `json:"files"`
	// PlanHash is the fingerprint of the plan the changeset claims to execute.
	PlanHash string `json:"plan_hash,omitempty"`
}

// planInput is the plan view: the plan fields promoted to the top level (so
// existing rules reading input.steps keep working) plus the view discriminator.
type planInput struct {
	plan.Plan
	View string `json:"view"`
}

type artifactInput struct {
	View     string   `json:"view"`
	Artifact Artifact `json:"artifact"`
}

type changesetInput struct {
	View      string    `json:"view"`
	Changeset Changeset `json:"changeset"`
}

// New builds a Linter from the ADR store. It populates the Registry from ADR
// metadata and compiles a single OPA PreparedEvalQuery over all paired .rego
// files found in adrDir. ADRs without a RegoFile (e.g. declarative-only ADRs)
// still contribute to the Registry but not to the Rego evaluation.
func New(store *adr.Store, adrDir string) (*Linter, error) {
	l := &Linter{
		Registry:      make(map[string]PolicyMeta),
		sessionSkills: make(map[string]map[string]*policy.Evaluator),
	}

	var regoPaths []string
	for _, inv := range store.Invariants {
		if inv.Enforcement.PolicyID == "" {
			continue
		}
		l.Registry[inv.Enforcement.PolicyID] = PolicyMeta{
			Nature:          Nature(inv.Nature),
			Rationale:       inv.Title,
			SunsetCondition: inv.SunsetCondition,
		}
		if inv.Enforcement.RegoFile != "" {
			regoPaths = append(regoPaths, filepath.Join(adrDir, inv.Enforcement.RegoFile))
		}
	}

	eval, err := policy.Prepare("data.ppg.linter.violation", regoPaths)
	if err != nil {
		return nil, err
	}
	l.eval = eval
	return l, nil
}

// Validate evaluates all Rego policies against the plan (plan view) and returns
// the violations. An empty slice means the plan can be locked. Unless
// AllowWideScope is set, it also enforces the built-in scope-breadth cap —
// a product rule rather than an ADR policy, so it is not part of Registry.
func (l *Linter) Validate(p *plan.Plan) []Violation {
	var violations []Violation
	if !l.AllowWideScope {
		violations = wideScopeViolations(p)
	}
	input := planInput{Plan: *p, View: "plan"}
	violations = append(violations, l.evaluateSkillCompanion(p.SessionID, p.SkillID, input)...)
	return append(violations, l.evaluate(input)...)
}

// LoadSkillCompanions registers the published skills found in dir (one
// subdirectory per skill holding SKILL.md and, for tier >= 1, SKILL.rego) and
// compiles each companion policy — Gate 3 of skill governance: a plan that
// declares skill_id is additionally evaluated against that skill's companion
// Rego, and an unknown skill_id rejects the plan.
func (l *Linter) LoadSkillCompanions(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading skills directory: %w", err)
	}
	loaded := make(map[string]*policy.Evaluator)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			continue // not a skill package
		}
		name := skillName(filepath.Join(skillDir, "SKILL.md"), e.Name())
		regoFile := filepath.Join(skillDir, "SKILL.rego")
		if _, err := os.Stat(regoFile); err != nil {
			loaded[name] = nil // published without a companion
			continue
		}
		ev, err := compileSkillFromFile(regoFile)
		if err != nil {
			return fmt.Errorf("skill %s: %w", name, err)
		}
		loaded[name] = ev
	}
	l.skillMu.Lock()
	l.skillCompanions = loaded
	l.skillMu.Unlock()
	return nil
}

// SkillCount reports how many published skills the operator loaded via
// -skills (the operator-provided tier). Session-scoped registrations do not
// count.
func (l *Linter) SkillCount() int {
	l.skillMu.RLock()
	defer l.skillMu.RUnlock()
	return len(l.skillCompanions)
}

// RegisterSessionSkill compiles rego (may be empty for a tier-0 skill) and
// stores the evaluator under (sessionID, name). Subsequent evaluate calls
// with that session id find the skill in the session-scoped tier; the
// operator tier still wins on name collision.
//
// A compile error is returned to the caller so the client sees the failure
// synchronously (POST /register_skill → 422 SKILL_COMPILE_ERROR). No state
// is mutated on error.
func (l *Linter) RegisterSessionSkill(sessionID, name, rego string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if name == "" {
		return fmt.Errorf("name is required")
	}
	ev, err := compileSkillFromModule(name, rego)
	if err != nil {
		return err
	}
	l.skillMu.Lock()
	if _, shadowed := l.skillCompanions[name]; shadowed {
		// Registration succeeds (idempotent client behavior) but the upload
		// will never be consulted — say so instead of shadowing silently.
		log.Printf("register_skill: session %s upload of %q is shadowed by the operator-provided skill of the same name (operator tier wins)",
			sessionID, name)
	}
	if l.sessionSkills[sessionID] == nil {
		l.sessionSkills[sessionID] = make(map[string]*policy.Evaluator)
	}
	l.sessionSkills[sessionID][name] = ev
	l.skillMu.Unlock()
	return nil
}

// AdoptSessions copies the session-scoped skill registrations from old into
// l. Used by the hot-reload path (SIGHUP on the validation server):
// rebuilding the durable corpus must not evict the skills that live
// sessions uploaded via /register_skill.
func (l *Linter) AdoptSessions(old *Linter) {
	if old == nil {
		return
	}
	old.skillMu.RLock()
	defer old.skillMu.RUnlock()
	l.skillMu.Lock()
	defer l.skillMu.Unlock()
	for sid, bySkill := range old.sessionSkills {
		copied := make(map[string]*policy.Evaluator, len(bySkill))
		maps.Copy(copied, bySkill)
		l.sessionSkills[sid] = copied
	}
}

// UnregisterSession drops every skill registered under sessionID. Called
// at session teardown; a no-op for an unknown session.
func (l *Linter) UnregisterSession(sessionID string) {
	l.skillMu.Lock()
	delete(l.sessionSkills, sessionID)
	l.skillMu.Unlock()
}

// SessionSkillCount reports how many skills are registered for sessionID.
// Diagnostic helper; tests use it to observe register/unregister effects.
func (l *Linter) SessionSkillCount(sessionID string) int {
	l.skillMu.RLock()
	defer l.skillMu.RUnlock()
	return len(l.sessionSkills[sessionID])
}

// compileSkillFromFile prepares an evaluator over a SKILL.rego on disk. Used
// at startup by LoadSkillCompanions.
func compileSkillFromFile(regoFile string) (*policy.Evaluator, error) {
	pkg, err := regoPackage(regoFile)
	if err != nil {
		return nil, err
	}
	return policy.Prepare("data."+pkg+".violation", []string{regoFile})
}

// compileSkillFromModule prepares an evaluator over an in-memory rego source.
// An empty source is a tier-0 skill (returns a ready no-op).
func compileSkillFromModule(name, source string) (*policy.Evaluator, error) {
	if source == "" {
		return nil, nil
	}
	pkg, err := regoPackageFromSource(source)
	if err != nil {
		return nil, err
	}
	return policy.PrepareModule("data."+pkg+".violation", name+".rego", source)
}

// lookupSkill resolves a skill by (sessionID, skillID) with operator-provided
// entries winning over session-scoped ones. Returns (evaluator, found).
func (l *Linter) lookupSkill(sessionID, skillID string) (*policy.Evaluator, bool) {
	l.skillMu.RLock()
	defer l.skillMu.RUnlock()
	if ev, ok := l.skillCompanions[skillID]; ok {
		return ev, true
	}
	if bySkill, ok := l.sessionSkills[sessionID]; ok {
		if ev, ok := bySkill[skillID]; ok {
			return ev, true
		}
	}
	return nil, false
}

// evaluateSkillCompanion evaluates the declared skill's companion Rego against
// one input document (any view). It fails closed: an unknown skill id —
// including every skill id when the gateway was started without -skills — is
// itself a violation. An empty skillID means the caller did not declare a
// skill, in which case only the ADR corpus applies. A registered skill with a
// nil evaluator (tier-0 publication) is a no-op.
func (l *Linter) evaluateSkillCompanion(sessionID, skillID string, input any) []Violation {
	if skillID == "" {
		return nil
	}
	ev, ok := l.lookupSkill(sessionID, skillID)
	if !ok {
		return []Violation{{
			PolicyID: "unknown_skill",
			Message: fmt.Sprintf("plan declares skill_id %q but no published skill with that name is registered with the gateway for session %q "+
				"(register it via POST /register_skill, or start ppg with -skills pointing at the published skills directory)", skillID, sessionID),
			Nature: Amplifier,
		}}
	}
	if ev == nil {
		return nil
	}
	violations, err := policy.Eval[Violation](ev, input)
	if err != nil {
		return []Violation{{
			PolicyID: "linter_eval_error",
			Message:  err.Error(),
			Nature:   Compensatory,
		}}
	}
	return violations
}

// skillName extracts the front-matter name of a SKILL.md, falling back to the
// directory name when absent.
func skillName(mdPath, fallback string) string {
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return fallback
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if after, ok := strings.CutPrefix(line, "name:"); ok {
			if name := strings.TrimSpace(after); name != "" {
				return name
			}
		}
	}
	return fallback
}

// regoPackage returns the package path declared by a .rego file.
func regoPackage(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	pkg, ok := scanPackage(string(raw))
	if !ok {
		return "", fmt.Errorf("%s: no package declaration", path)
	}
	return pkg, nil
}

// regoPackageFromSource returns the package path declared by an in-memory
// rego source. Same syntax as a file — the compiler is oblivious to the
// origin.
func regoPackageFromSource(source string) (string, error) {
	pkg, ok := scanPackage(source)
	if !ok {
		return "", fmt.Errorf("no package declaration in rego source")
	}
	return pkg, nil
}

func scanPackage(source string) (string, bool) {
	for line := range strings.SplitSeq(source, "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "package "); ok {
			return strings.TrimSpace(after), true
		}
	}
	return "", false
}

// wideScopeViolations rejects step targets so broad that the ticket derived
// from them would be allow-all (deny-by-default cap on scope breadth).
func wideScopeViolations(p *plan.Plan) []Violation {
	var violations []Violation
	for _, s := range p.Steps {
		for _, t := range s.Targets {
			if isWideTarget(t) {
				violations = append(violations, Violation{
					PolicyID: "scope_breadth_cap",
					Message: fmt.Sprintf("step %q: target %q is too broad — the derived ticket would allow modifying the whole repository. "+
						"Enumerate the files or directories the step actually touches (operators can restore the old behavior with ppg -allow-wide-scope).",
						s.ID, t),
					Nature: Amplifier,
				})
			}
		}
	}
	return violations
}

// isWideTarget reports whether a target grants an effectively unlimited file
// scope. A trailing "*" is a prefix pattern in the ticket scope, so the check
// applies to the prefix that remains once wildcards are stripped.
func isWideTarget(target string) bool {
	t := strings.TrimSpace(target)
	if t == "" {
		return true
	}
	prefix := strings.TrimRight(t, "*")
	if prefix == "" { // "*", "**"
		return true
	}
	clean := path.Clean(prefix)
	if clean == "." || clean == ".." || clean == "/" {
		return true
	}
	return strings.HasPrefix(clean, "../")
}

// EvaluateArtifact evaluates the corpus against a single edited file's content
// (artifact view) — the in-loop check behind the guard hook and Smart Tools.
// The declared skill (when skillID is non-empty) is evaluated fail-closed —
// an unknown id is itself a violation. Independently, EVERY registered skill
// applicable to the session (operator tier + this session's uploads) is
// unioned in: an installed skill's content invariants apply automatically,
// whether or not the plan declared that skill (union semantics — a plan that
// omits skill_id no longer bypasses skill content rules). sessionID selects
// the session-scoped tier; pass "" to consult only the operator tier.
func (l *Linter) EvaluateArtifact(sessionID, skillID string, a Artifact) []Violation {
	input := artifactInput{View: "artifact", Artifact: a}
	violations := l.evaluateSkillCompanion(sessionID, skillID, input)
	violations = append(violations, l.evaluateAllSkills(sessionID, skillID, input)...)
	return append(violations, l.evaluate(input)...)
}

// EvaluateChangeset evaluates the corpus against a whole diff (changeset view) —
// the apply-time backstop. Same union semantics as EvaluateArtifact: the
// declared skill is evaluated fail-closed, and every other registered skill
// applicable to the session is unioned in regardless of the declared id.
func (l *Linter) EvaluateChangeset(sessionID, skillID string, c Changeset) []Violation {
	input := changesetInput{View: "changeset", Changeset: c}
	violations := l.evaluateSkillCompanion(sessionID, skillID, input)
	violations = append(violations, l.evaluateAllSkills(sessionID, skillID, input)...)
	return append(violations, l.evaluate(input)...)
}

// evaluateAllSkills evaluates every registered skill companion applicable to
// the session — the operator tier plus this session's uploads, the operator
// winning on name collision — against one content-view input document,
// skipping the declared skill (already evaluated fail-closed by
// evaluateSkillCompanion). This implements the governed-machine intent for
// the content altitudes: as soon as an installed skill provides a
// validation, it applies automatically. Plan-view rules stay selected by
// skill_id: a skill's *workflow requirements* (e.g. "the plan must read the
// tokens file") only make sense for plans executed under that skill, whereas
// its *content invariants* (artifact/changeset views) hold for every edit.
// Iteration is name-sorted so verdicts are reproducible run to run.
func (l *Linter) evaluateAllSkills(sessionID, declaredSkillID string, input any) []Violation {
	l.skillMu.RLock()
	evs := make(map[string]*policy.Evaluator, len(l.skillCompanions)+len(l.sessionSkills[sessionID]))
	for name, ev := range l.sessionSkills[sessionID] {
		evs[name] = ev
	}
	for name, ev := range l.skillCompanions { // operator tier wins on collision
		evs[name] = ev
	}
	l.skillMu.RUnlock()

	var violations []Violation
	for _, name := range slices.Sorted(maps.Keys(evs)) {
		if name == declaredSkillID {
			continue
		}
		ev := evs[name]
		if ev == nil {
			continue // tier-0 skill: registered, no companion policy
		}
		vs, err := policy.Eval[Violation](ev, input)
		if err != nil {
			violations = append(violations, Violation{
				PolicyID: "linter_eval_error",
				Message:  fmt.Sprintf("skill %s: %v", name, err),
				Nature:   Compensatory,
			})
			continue
		}
		violations = append(violations, vs...)
	}
	return violations
}

// evaluate runs the corpus against one input document. It fails closed: an
// evaluation or decode error surfaces as a synthetic rejection rather than a
// silent pass.
func (l *Linter) evaluate(input any) []Violation {
	violations, err := policy.Eval[Violation](l.eval, input)
	if err != nil {
		return []Violation{{
			PolicyID: "linter_eval_error",
			Message:  err.Error(),
			Nature:   Compensatory,
		}}
	}
	return violations
}
