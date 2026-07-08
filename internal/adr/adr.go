// Package adr loads Architecture Decision Records (ADRs) and retrieves the
// architectural invariants relevant to an intent.
//
// The ADR store is the declarative amplifier core of the platform: it holds
// semantic invariants (not step-by-step recipes) that the agent reasons over.
package adr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Enforcement describes how an invariant is enforced.
type Enforcement struct {
	// Mode is the enforcement mechanism (e.g. "programmatic", "declarative").
	Mode string `yaml:"mode" json:"mode"`
	// PolicyID is the linter policy that implements this invariant. Only set
	// when Mode is "programmatic"; matches a key in linter.Registry.
	PolicyID string `yaml:"policy_id" json:"policy_id,omitempty"`
	// RegoFile is the filename (relative to the ADR directory) of the Rego
	// policy that deterministically enforces this invariant at lock_in_plan
	// time. Only set when Mode is "programmatic". The semantic directive
	// (InvariantText) serves at enrich() time; this file serves at lock time.
	RegoFile string `yaml:"rego" json:"rego,omitempty"`
}

// Invariant is one architectural invariant, parsed from an ADR file.
type Invariant struct {
	// ADRID is the unique identifier of the source ADR (e.g. "ADR-042").
	ADRID string `yaml:"adr_id" json:"adr_id"`
	// Title is the human-readable name of the invariant.
	Title string `yaml:"title" json:"title"`
	// Status reflects the ADR lifecycle state (e.g. "accepted", "deprecated").
	Status string `yaml:"status" json:"status"`
	// Nature positions the invariant on the durability axis:
	// "amplifier" for durable decisions, "compensatory" for temporary scaffolding.
	Nature string `yaml:"nature" json:"nature"`
	// SunsetCondition is the measurable condition under which a compensatory
	// invariant can be removed. Empty for amplifier invariants.
	SunsetCondition string `yaml:"sunset_condition" json:"sunset_condition,omitempty"`
	// ScopeSelectors are the keywords used to match this invariant to an
	// intent during retrieval (see Store.Retrieve).
	ScopeSelectors []string `yaml:"scope_selectors" json:"scope_selectors"`
	// Enforcement describes the mechanism that enforces this invariant.
	Enforcement Enforcement `yaml:"enforcement" json:"enforcement"`
	// InvariantText is the body of the ADR Markdown file (everything after
	// the YAML front matter). Injected into the agent's planning context.
	InvariantText string `yaml:"-" json:"invariant"`
}

// Store holds all invariants loaded from an ADR directory.
type Store struct {
	// Invariants is the complete set of architectural invariants available
	// for retrieval. Populated by Load; never modified after construction.
	Invariants []Invariant
}

// Load reads every *.md file of dir and parses its YAML front matter.
func Load(dir string) (*Store, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	s := &Store{}
	for _, f := range files {
		inv, err := parseFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		s.Invariants = append(s.Invariants, inv)
	}
	return s, nil
}

func parseFile(path string) (Invariant, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Invariant{}, err
	}
	content := string(raw)
	if !strings.HasPrefix(content, "---\n") {
		return Invariant{}, fmt.Errorf("missing YAML front matter")
	}
	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return Invariant{}, fmt.Errorf("unterminated YAML front matter")
	}
	var inv Invariant
	if err := yaml.Unmarshal([]byte(parts[0]), &inv); err != nil {
		return Invariant{}, err
	}
	inv.InvariantText = strings.TrimSpace(parts[1])
	return inv, nil
}

// Retrieve returns the invariants whose scope selectors match the intent.
// PoC implementation: keyword matching. Production: embeddings + reranking.
// The key property holds either way: no business pattern is hard-coded here —
// architects declare selectors in the ADRs themselves.
func (s *Store) Retrieve(intent string) []Invariant {
	intentLow := strings.ToLower(intent)
	var matched []Invariant
	for _, inv := range s.Invariants {
		for _, sel := range inv.ScopeSelectors {
			if strings.Contains(intentLow, strings.ToLower(sel)) {
				matched = append(matched, inv)
				break
			}
		}
	}
	return matched
}
