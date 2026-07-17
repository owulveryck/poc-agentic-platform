// Package plan defines the structured plan contract the agent must materialize
// before any execution tool will work (schemas/plan.schema.json is the
// language-neutral version of this contract).
package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// RepoContext describes the repository the agent works on.
type RepoContext struct {
	// Name is the canonical repository name (e.g. "payments-service").
	Name string `json:"name"`
	// TechStack lists the programming languages and frameworks in use
	// (e.g. ["Go", "PostgreSQL"]). Used by linter policies to apply
	// language-specific rules.
	TechStack []string `json:"tech_stack"`
	// CurrentBranch is the git branch the agent is working on. Optional;
	// used for audit purposes.
	CurrentBranch string `json:"current_branch,omitempty"`
}

// Step is one node of the agent's execution graph.
type Step struct {
	// ID is a unique identifier for this step within the plan (e.g. "step-1").
	ID string `json:"id"`
	// Action is a natural-language description of what this step does
	// (e.g. "Add user authentication middleware").
	Action string `json:"action"`
	// Tool is the Smart Platform Tool to invoke for this step
	// (e.g. "patch_code", "apply_db_migration", "go-test").
	Tool string `json:"tool"`
	// Targets is the list of file paths or database objects the tool will act
	// on. These are extracted to form the least-privilege scope of the ticket.
	Targets []string `json:"targets"`
	// DependsOn lists IDs of steps that must complete before this one runs.
	// Omit for steps with no prerequisites.
	DependsOn []string `json:"depends_on,omitempty"`
}

// Plan is the structured plan the agent submits to lock_in_plan.
type Plan struct {
	// SessionID is a unique identifier for this planning session. Used as the
	// JWT subject so every ticket can be traced back to the originating plan.
	SessionID string `json:"session_id"`
	// StreamAlignedTeam is the team that owns this work. Optional; used for
	// audit and routing.
	StreamAlignedTeam string `json:"stream_aligned_team,omitempty"`
	// Intent is the natural-language description of what the agent is trying
	// to achieve. Must be at least 5 characters. Used by the enrich endpoint
	// to retrieve relevant architectural invariants.
	Intent string `json:"intent"`
	// RepositoryContext describes the target repository.
	RepositoryContext RepoContext `json:"repository_context"`
	// Steps is the ordered, acyclic execution graph the agent will follow.
	// Must contain at least one step.
	Steps []Step `json:"steps"`
}

// ValidateStructure enforces the structural contract (the JSON-Schema part).
func (p *Plan) ValidateStructure() error {
	if p.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if len(p.Intent) < 5 {
		return fmt.Errorf("intent must be at least 5 characters")
	}
	if p.RepositoryContext.Name == "" {
		return fmt.Errorf("repository_context.name is required")
	}
	if len(p.RepositoryContext.TechStack) == 0 {
		return fmt.Errorf("repository_context.tech_stack is required")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("plan must contain at least one step")
	}
	ids := make(map[string]bool, len(p.Steps))
	for i, s := range p.Steps {
		if s.ID == "" || s.Action == "" || s.Tool == "" || len(s.Targets) == 0 {
			return fmt.Errorf("step %d: id, action, tool and targets are required", i)
		}
		if ids[s.ID] {
			return fmt.Errorf("step %d: duplicate step id %q", i, s.ID)
		}
		ids[s.ID] = true
	}
	// The Steps form an acyclic dependency graph: every DependsOn must name an
	// existing step, and the graph must contain no cycles. Without this a plan
	// could reference a phantom prerequisite or lock a ticket for a mutually
	// recursive graph that can never execute in order.
	for i, s := range p.Steps {
		for _, dep := range s.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("step %d (%q): depends_on references unknown step %q", i, s.ID, dep)
			}
			if dep == s.ID {
				return fmt.Errorf("step %d (%q): depends_on references itself", i, s.ID)
			}
		}
	}
	if cycle := p.findDependencyCycle(); cycle != "" {
		return fmt.Errorf("plan dependency graph has a cycle involving step %q", cycle)
	}
	return nil
}

// findDependencyCycle returns the ID of a step involved in a dependency cycle,
// or "" if the DependsOn graph is acyclic. It assumes every DependsOn target
// exists (validated by the caller) and detects cycles with a depth-first
// three-color walk.
func (p *Plan) findDependencyCycle() string {
	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS stack
		black = 2 // fully explored
	)
	byID := make(map[string]Step, len(p.Steps))
	for _, s := range p.Steps {
		byID[s.ID] = s
	}
	color := make(map[string]int, len(p.Steps))
	var visit func(id string) string
	visit = func(id string) string {
		color[id] = gray
		for _, dep := range byID[id].DependsOn {
			switch color[dep] {
			case gray:
				return dep
			case white:
				if c := visit(dep); c != "" {
					return c
				}
			}
		}
		color[id] = black
		return ""
	}
	for _, s := range p.Steps {
		if color[s.ID] == white {
			if c := visit(s.ID); c != "" {
				return c
			}
		}
	}
	return ""
}

// Hash returns the canonical SHA-256 fingerprint of the plan. It is embedded
// in the capability ticket so execution tools can detect plan substitution.
// A marshal failure is returned rather than swallowed: a fingerprint used as
// a security claim must never silently degrade to the hash of nil.
func (p *Plan) Hash() (string, error) {
	canonical, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("canonicalizing plan for hashing: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// HasTech reports whether the repository declares the given technology.
func (p *Plan) HasTech(tech string) bool {
	for _, t := range p.RepositoryContext.TechStack {
		if t == tech {
			return true
		}
	}
	return false
}
