// Package ticket issues and verifies capability tickets.
//
// A capability ticket is an ephemeral signed JWT derived from a validated
// plan: it embeds the plan fingerprint and the least-privilege scope (files
// and tools) the agent is allowed to use. Every Smart Platform Tool verifies
// it before acting, which bounds the blast radius of systemic errors — a
// guardrail that stays relevant even if the model were perfect.
package ticket

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

// EnvSecret names the environment variable that, when set, becomes the HS256
// signing key verbatim. It takes precedence over UseKeyFile.
const EnvSecret = "PPG_TICKET_SECRET"

// secret is the HS256 signing key. It is never hardcoded: it comes from
// EnvSecret, from the per-machine key file (UseKeyFile), or — for processes
// that configure neither, such as tests — from a random per-process key.
// A per-process key still verifies everything this process signed, and a
// validation server restart simply invalidates outstanding tickets (fail closed).
// Production posture remains asymmetric keys behind a KMS, with rotation.
var secret = initialSecret()

func initialSecret() []byte {
	if v := os.Getenv(EnvSecret); v != "" {
		return []byte(v)
	}
	b := make([]byte, 32)
	rand.Read(b) //nolint:errcheck // never fails (crypto/rand panics instead since Go 1.24)
	return b
}

// UseKeyFile installs the hex-encoded key stored at path as the signing key,
// generating and persisting a fresh 32-byte key (0600, parent 0700) on first
// run. When EnvSecret is set it wins and the file is neither read nor written.
// The validation server calls this at startup so tickets survive restarts on the same
// machine.
func UseKeyFile(path string) error {
	if os.Getenv(EnvSecret) != "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		key := make([]byte, 32)
		rand.Read(key) //nolint:errcheck // see initialSecret
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("creating key directory: %w", err)
		}
		if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
			return fmt.Errorf("persisting signing key: %w", err)
		}
		secret = key
		return nil
	case err != nil:
		return fmt.Errorf("reading signing key %s: %w", path, err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return fmt.Errorf("signing key %s is not hex: %w", path, err)
	}
	if len(key) < 32 {
		return fmt.Errorf("signing key %s is too short (%d bytes, need 32)", path, len(key))
	}
	secret = key
	return nil
}

// DefaultTTL is the wall-clock lifetime stamped on a ticket when the caller
// does not specify one. It is a defense-in-depth CAP, not the primary bound:
// since ADR-100 the capability already dies with its session — SessionStart
// purges the TokenStore and the session_id claim is checked on every hook — so
// a leaked ticket is useless in any other session. The wall-clock cap only
// bounds a same-session leak, so it defaults to a working session rather than
// the old 15 minutes (which fired shorter than a real session and forced a
// re-lock mid-task). Operators tighten or loosen it via the validation server's
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
	// SkillID names the published skill the plan declared, or empty when the
	// plan was locked without a skill. Content-view gates (artifact, changeset)
	// use it to evaluate that skill's companion Rego alongside the ADR corpus.
	SkillID string `json:"skill_id,omitempty"`
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
	planHash, err := p.Hash()
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := Claims{
		SessionID: p.SessionID,
		PlanHash:  planHash,
		SkillID:   p.SkillID,
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
