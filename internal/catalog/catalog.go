// Package catalog loads the platform Service Catalog and retrieves the services
// that provide a capability an agent needs.
//
// The catalog is the discovery counterpart of the ADR store (internal/adr):
// where the ADR store answers "which invariants apply to this intent?", the
// catalog answers "which sanctioned service should I build on for this
// capability?". Records are declarative — one Markdown file per service, YAML
// front matter for the metadata plus a body that documents how to call the
// service — and carry graph-like relations (supersedes / superseded_by /
// alternative_to), so the catalog is a small knowledge graph of the org's
// capabilities. Ranking ("which is the best one?") is policy-as-code and lives
// in ranker.go; this file is only loading and retrieval.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Status values a service record may declare. The ranking policy
// (service-policy/*.rego) decides what each one means for recommendation; these
// constants document the vocabulary.
const (
	StatusRecommended = "recommended"
	StatusAllowed     = "allowed"
	StatusSandbox     = "sandbox"
	StatusDeprecated  = "deprecated"
	StatusForbidden   = "forbidden"
)

// Service is one entry in the catalog, parsed from a services/*.md file.
type Service struct {
	// ServiceID is the unique identifier (e.g. "notify-svc").
	ServiceID string `yaml:"service_id" json:"service_id"`
	// Name is the human-readable service name.
	Name string `yaml:"name" json:"name"`
	// Capability is the category this service provides (e.g. "notification",
	// "payment"). Retrieval matches on it directly.
	Capability string `yaml:"capability" json:"capability"`
	// Status positions the service for the ranking policy: recommended, allowed,
	// sandbox, deprecated, or forbidden.
	Status string `yaml:"status" json:"status"`
	// Tier is a priority hint used as a ranking tie-break (lower = higher
	// priority). Optional.
	Tier int `yaml:"tier" json:"tier"`
	// Endpoint is where the service is reached (base URL). For the demo, a local
	// mock (e.g. http://localhost:9110).
	Endpoint string `yaml:"endpoint" json:"endpoint,omitempty"`
	// OwnerTeam is the team accountable for the service.
	OwnerTeam string `yaml:"owner_team" json:"owner_team,omitempty"`
	// Selectors are keywords matched (case-insensitive substring) against an
	// intent when discovery is intent-driven rather than capability-driven.
	Selectors []string `yaml:"selectors" json:"selectors,omitempty"`
	// Supersedes lists service IDs this one replaces (graph edge).
	Supersedes []string `yaml:"supersedes" json:"supersedes,omitempty"`
	// SupersededBy names the service that replaces this one (graph edge); set on
	// deprecated records so discovery can point at the successor.
	SupersededBy string `yaml:"superseded_by" json:"superseded_by,omitempty"`
	// AlternativeTo lists sibling service IDs providing the same capability
	// (graph edge).
	AlternativeTo []string `yaml:"alternative_to" json:"alternative_to,omitempty"`
	// PolicyTags carries free-form attributes the ranking policy may read
	// (region, compliance, cost, sla, …).
	PolicyTags map[string]string `yaml:"policy_tags" json:"policy_tags,omitempty"`
	// APIUsage is the Markdown body of the record: how to call the service.
	// Returned to the agent so it uses the real API instead of guessing.
	APIUsage string `yaml:"-" json:"api_usage"`
}

// Store holds all services loaded from a catalog directory.
type Store struct {
	// Services is the complete catalog, in load order. Populated by Load.
	Services []Service
}

// Load reads every *.md file of dir and parses its YAML front matter + body.
func Load(dir string) (*Store, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	s := &Store{}
	for _, f := range files {
		svc, err := parseFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		s.Services = append(s.Services, svc)
	}
	return s, nil
}

func parseFile(path string) (Service, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Service{}, err
	}
	content := string(raw)
	if !strings.HasPrefix(content, "---\n") {
		return Service{}, fmt.Errorf("missing YAML front matter")
	}
	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return Service{}, fmt.Errorf("unterminated YAML front matter")
	}
	var svc Service
	if err := yaml.Unmarshal([]byte(parts[0]), &svc); err != nil {
		return Service{}, err
	}
	if svc.ServiceID == "" {
		return Service{}, fmt.Errorf("service_id is required")
	}
	if svc.Capability == "" {
		return Service{}, fmt.Errorf("capability is required")
	}
	svc.APIUsage = strings.TrimSpace(parts[1])
	return svc, nil
}

// Retrieve returns the candidate services for a capability. When capability is
// non-empty it filters by exact (case-insensitive) capability; otherwise it
// falls back to matching the intent against each service's capability or its
// selectors (case-insensitive substring) — the same keyword semantics as
// adr.Store.Retrieve. Ranking is applied separately by the Ranker.
func (s *Store) Retrieve(capability, intent string) []Service {
	capLow := strings.ToLower(strings.TrimSpace(capability))
	intentLow := strings.ToLower(intent)
	var out []Service
	for _, svc := range s.Services {
		switch {
		case capLow != "":
			if strings.EqualFold(svc.Capability, capLow) {
				out = append(out, svc)
			}
		case svc.Capability != "" && strings.Contains(intentLow, strings.ToLower(svc.Capability)):
			out = append(out, svc)
		default:
			for _, sel := range svc.Selectors {
				if sel != "" && strings.Contains(intentLow, strings.ToLower(sel)) {
					out = append(out, svc)
					break
				}
			}
		}
	}
	return out
}

// Get returns the service with the given id, or false if absent.
func (s *Store) Get(id string) (Service, bool) {
	for _, svc := range s.Services {
		if svc.ServiceID == id {
			return svc, true
		}
	}
	return Service{}, false
}

// All returns every service in the catalog.
func (s *Store) All() []Service { return s.Services }
