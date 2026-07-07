// Package ticket issues and verifies capability tickets.
//
// A capability ticket is an ephemeral signed JWT derived from a validated
// plan: it embeds the plan fingerprint and the least-privilege scope (files
// and tools) the agent is allowed to use. Every Smart Platform Tool verifies
// it before acting, which bounds the blast radius of systemic errors — a
// guardrail that stays relevant even if the model were perfect.
package ticket

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

// secret is the PoC signing key. Production: asymmetric keys behind a KMS,
// with rotation. Never ship a symmetric hard-coded secret.
var secret = []byte("poc-secret-rotate-me")

// TTL is the ticket lifetime: short by design, the ticket dies with the task.
const TTL = 15 * time.Minute

// Scope is the least-privilege contract derived from the locked plan.
type Scope struct {
	// AllowModify is the set of file paths (or path prefixes ending in *)
	// the ticket holder is permitted to modify. Derived from plan step targets.
	AllowModify []string `json:"allow_modify"`
	// AllowTool is the set of Smart Tool IDs the ticket holder may invoke.
	// Derived from plan step tool fields.
	AllowTool []string `json:"allow_tool"`
}

// Claims are the ticket claims carried by the JWT.
type Claims struct {
	// SessionID matches the plan's session_id, tying every downstream
	// verification back to the originating planning session.
	SessionID string `json:"session_id"`
	// PlanHash is the SHA-256 fingerprint of the locked plan. Execution tools
	// can compare it against the current plan to detect plan substitution.
	PlanHash string `json:"plan_hash"`
	// Scope is the least-privilege contract the agent must stay within.
	Scope Scope `json:"scope"`
	jwt.RegisteredClaims
}

// DeriveScope computes the allowed files and tools from the plan steps.
func DeriveScope(p *plan.Plan) Scope {
	seenFile, seenTool := map[string]bool{}, map[string]bool{}
	var scope Scope
	for _, s := range p.Steps {
		if !seenTool[s.Tool] {
			seenTool[s.Tool] = true
			scope.AllowTool = append(scope.AllowTool, s.Tool)
		}
		for _, t := range s.Targets {
			if !seenFile[t] {
				seenFile[t] = true
				scope.AllowModify = append(scope.AllowModify, t)
			}
		}
	}
	return scope
}

// Issue signs a capability ticket for a validated plan.
func Issue(p *plan.Plan) (string, error) {
	now := time.Now()
	claims := Claims{
		SessionID: p.SessionID,
		PlanHash:  p.Hash(),
		Scope:     DeriveScope(p),
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(TTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// Verify decodes and checks a ticket; used in-tool by every Smart Tool.
func Verify(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("invalid ticket")
	}
	return claims, nil
}
