// Package skill handles the parsing and governance validation of Claude Code
// skills. A skill is a SKILL.md file (YAML front matter + Markdown body) that
// encapsulates a reusable agentic workflow. Enterprise skills are dual-
// representation governance artifacts: the Markdown body is the semantic
// directive consumed at invocation time; the companion SKILL.rego is the
// deterministic enforcement policy validated at publish time.
package skill

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill holds the parsed content of a SKILL.md file.
type Skill struct {
	// Name is the skill identifier, used as the /skill-name trigger.
	Name string `yaml:"name" json:"name"`
	// Description is the primary discovery mechanism. It must encode both what
	// the skill does and when to use it.
	Description string `yaml:"description" json:"description"`
	// Version is the semver version, required for registry publication.
	Version string `yaml:"version" json:"version,omitempty"`
	// ArgumentHint documents the expected arguments, shown in /help.
	// Required when the skill body uses $ARGUMENTS.
	ArgumentHint string `yaml:"argument-hint" json:"argument_hint,omitempty"`
	// Body is the Markdown body of the skill (everything after the front matter).
	// It contains the instructions the agent follows when the skill is invoked.
	Body string `yaml:"-" json:"body"`
	// RegoPolicy is the content of the companion SKILL.rego file, if present.
	// Required for skills that instruct file modifications (tier ≥ 1).
	RegoPolicy string `yaml:"-" json:"rego_policy,omitempty"`
}

// Parse builds a Skill from the raw bytes of a SKILL.md file and an optional
// companion SKILL.rego. Either or both arguments may be nil.
func Parse(skillMD []byte, regoPolicy []byte) (*Skill, error) {
	content := string(skillMD)
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("missing YAML front matter")
	}
	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("unterminated YAML front matter")
	}
	var s Skill
	if err := yaml.Unmarshal([]byte(parts[0]), &s); err != nil {
		return nil, err
	}
	s.Body = strings.TrimSpace(parts[1])
	if len(regoPolicy) > 0 {
		s.RegoPolicy = string(regoPolicy)
	}
	return &s, nil
}
