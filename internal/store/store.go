// Package store exposes the durable homes for PPG's per-project mutable
// state: the current agent session id (SessionStore) and the capability
// tickets bound to those sessions (TokenStore). Both are per-project,
// keyed to the normalized absolute project directory.
//
// The default implementation is Filesystem, rooted at $XDG_STATE_HOME/ppg
// (fallback ~/.local/state/ppg). The two stores share layout and lifecycle
// helpers, which is why they live in the same package.
package store

import "errors"

// ErrNotFound is the sentinel returned by Get/GetActive when the key is
// absent. Callers use errors.Is to distinguish "no ticket yet" from a
// real I/O failure.
var ErrNotFound = errors.New("store: not found")

// TokenStore holds capability tickets keyed by session id. Values are
// opaque strings; the store knows nothing about JWT semantics — that
// stays in internal/ticket and internal/smarttools.
type TokenStore interface {
	// Put persists ticket under sessionID, overwriting any previous value.
	Put(sessionID, ticket string) error
	// Get returns the ticket for sessionID, or ErrNotFound when absent.
	Get(sessionID string) (string, error)
	// Delete removes the ticket for sessionID. Missing keys are not an
	// error: callers only care that the state is gone.
	Delete(sessionID string) error
	// Reset removes every ticket for the project. Used at SessionStart to
	// ensure a capability never survives the session that locked it.
	Reset() error
}

// SessionStore holds the single "active session id" for the project.
type SessionStore interface {
	// PutActive records sessionID as the current active session.
	PutActive(sessionID string) error
	// GetActive returns the active session id, or ErrNotFound when unset.
	GetActive() (string, error)
	// ClearActive removes any active session marker. Idempotent.
	ClearActive() error
}
