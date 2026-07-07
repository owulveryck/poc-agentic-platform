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
	Name          string   `json:"name"`
	TechStack     []string `json:"tech_stack"`
	CurrentBranch string   `json:"current_branch,omitempty"`
}

// Step is one node of the agent's execution graph.
type Step struct {
	ID        string   `json:"id"`
	Action    string   `json:"action"`
	Tool      string   `json:"tool"`
	Targets   []string `json:"targets"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// Plan is the structured plan the agent submits to lock_in_plan.
type Plan struct {
	SessionID         string      `json:"session_id"`
	StreamAlignedTeam string      `json:"stream_aligned_team,omitempty"`
	Intent            string      `json:"intent"`
	RepositoryContext RepoContext `json:"repository_context"`
	Steps             []Step      `json:"steps"`
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
	for i, s := range p.Steps {
		if s.ID == "" || s.Action == "" || s.Tool == "" || len(s.Targets) == 0 {
			return fmt.Errorf("step %d: id, action, tool and targets are required", i)
		}
	}
	return nil
}

// Hash returns the canonical SHA-256 fingerprint of the plan. It is embedded
// in the capability ticket so execution tools can detect plan substitution.
func (p *Plan) Hash() string {
	canonical, _ := json.Marshal(p)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
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
