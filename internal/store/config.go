package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// EnvStoreRoot is the env var that overrides the default store root.
const EnvStoreRoot = "PPG_STORE_ROOT"

// EnvProjectDir is the env var that identifies the project this process
// belongs to. Required for the MCP server (no cwd fallback), optional for
// hooks (which fall back to their payload's cwd or os.Getwd).
const EnvProjectDir = "PPG_PROJECT_DIR"

// DefaultRoot returns the default store root:
//
//	$XDG_STATE_HOME/ppg     when the env var is set
//	$HOME/.local/state/ppg  otherwise
//
// It returns an error when neither $XDG_STATE_HOME nor $HOME can be
// determined — better to fail loudly than to write state under cwd.
func DefaultRoot() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "ppg"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("store: cannot resolve $HOME for default root: %w", err)
	}
	if home == "" {
		return "", errors.New("store: $HOME is empty; set PPG_STORE_ROOT or XDG_STATE_HOME")
	}
	return filepath.Join(home, ".local", "state", "ppg"), nil
}

// ResolveRoot picks the store root using the precedence:
//
//	flagValue  >  $PPG_STORE_ROOT  >  DefaultRoot()
//
// Pass "" for flagValue when the caller did not receive an explicit flag.
// The returned path is normalized (Abs + EvalSymlinks + Clean).
func ResolveRoot(flagValue string) (string, error) {
	raw := flagValue
	if raw == "" {
		raw = os.Getenv(EnvStoreRoot)
	}
	if raw == "" {
		def, err := DefaultRoot()
		if err != nil {
			return "", err
		}
		raw = def
	}
	return Normalize(raw)
}

// ResolveProjectDir picks the project directory using the precedence:
//
//	flagValue  >  $PPG_PROJECT_DIR  >  fallback
//
// Pass "" for fallback to require an explicit value (the MCP server does
// this — its cwd is stale by design). Pass os.Getwd() or the hook payload
// cwd for permissive callers (the guards do this).
// The returned path is normalized.
func ResolveProjectDir(flagValue, fallback string) (string, error) {
	raw := flagValue
	if raw == "" {
		raw = os.Getenv(EnvProjectDir)
	}
	if raw == "" {
		raw = fallback
	}
	if raw == "" {
		return "", errors.New("store: project dir unresolved; pass --project-dir or set " + EnvProjectDir)
	}
	return Normalize(raw)
}

// Normalize turns a path into its canonical absolute form: Abs, then
// EvalSymlinks (tolerant to non-existent leaves — the store root usually
// does not exist on first invocation), then Clean.
func Normalize(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("store: abs %s: %w", p, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("store: evalsymlinks %s: %w", abs, err)
		}
		resolved = abs
	}
	return filepath.Clean(resolved), nil
}
