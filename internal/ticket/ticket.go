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

// DefaultTTL is the wall-clock lifetime stamped on a ticket when the caller
// does not specify one. It is a defense-in-depth CAP, not the primary bound:
// since ADR-100 the capability already dies with its session — SessionStart
// purges the TokenStore and the session_id claim is checked on every hook — so
// a leaked ticket is useless in any other session. The wall-clock cap only
// bounds a same-session leak, so it defaults to a working session rather than
// the old 15 minutes (which fired shorter than a real session and forced a
// re-lock mid-task). Operators tighten or loosen it via the gateway's
// -ticket-ttl flag / PPG_TICKET_TTL env.
var DefaultTTL = 8 * time.Hour

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

// Issue signs a capability ticket for a validated plan using DefaultTTL.
func Issue(p *plan.Plan) (string, error) {
	return IssueWithTTL(p, DefaultTTL)
}

// IssueWithTTL signs a capability ticket for a validated plan with an explicit
// wall-clock lifetime. A ttl <= 0 falls back to DefaultTTL. The session purge
// (ADR-100) remains the primary bound regardless of ttl.
func IssueWithTTL(p *plan.Plan, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := time.Now()
	claims := Claims{
		SessionID: p.SessionID,
		PlanHash:  p.Hash(),
		Scope:     DeriveScope(p),
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
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
