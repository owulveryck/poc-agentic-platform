package store

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Filesystem is the on-disk implementation of TokenStore and SessionStore.
// A single value implements both interfaces since they share the same
// per-project root and lifecycle.
type Filesystem struct {
	root       string // e.g. /Users/x/.local/state/ppg
	projectDir string // normalized absolute project path
	slug       string // base64.RawURLEncoding(projectDir)
}

// NewFilesystem builds a store rooted at root, scoped to projectDir.
// Both paths must be non-empty; projectDir is normalized (Abs + EvalSymlinks
// + Clean) before slugging. Directories on disk are created lazily on the
// first Put/PutActive so a read-only GetActive on a virgin project does
// not spuriously materialize state.
func NewFilesystem(root, projectDir string) (*Filesystem, error) {
	if root == "" {
		return nil, errors.New("store: root is empty")
	}
	if projectDir == "" {
		return nil, errors.New("store: project dir is empty")
	}
	normProject, err := Normalize(projectDir)
	if err != nil {
		return nil, fmt.Errorf("store: normalize project dir: %w", err)
	}
	normRoot, err := Normalize(root)
	if err != nil {
		return nil, fmt.Errorf("store: normalize root: %w", err)
	}
	return &Filesystem{
		root:       normRoot,
		projectDir: normProject,
		slug:       base64.RawURLEncoding.EncodeToString([]byte(normProject)),
	}, nil
}

// Root returns the absolute store root (mainly for debugging/tests).
func (f *Filesystem) Root() string { return f.root }

// ProjectDir returns the normalized absolute project dir.
func (f *Filesystem) ProjectDir() string { return f.projectDir }

func (f *Filesystem) projectRoot() string {
	return filepath.Join(f.root, "projects", f.slug)
}

func (f *Filesystem) sessionPath() string {
	return filepath.Join(f.projectRoot(), "session")
}

func (f *Filesystem) ticketDir() string {
	return filepath.Join(f.projectRoot(), "tickets")
}

func (f *Filesystem) ticketPath(sid string) string {
	return filepath.Join(f.ticketDir(), sid)
}

// lockPath is the per-project advisory lock file. It lives at the project root
// (a sibling of the session file and tickets dir), never inside ticketDir, so
// Reset's RemoveAll does not delete it.
func (f *Filesystem) lockPath() string {
	return filepath.Join(f.projectRoot(), ".lock")
}

// withLock serializes store operations on this project across processes (and
// goroutines) via an advisory flock on lockPath. Writers pass exclusive=true
// (LOCK_EX); readers pass false (LOCK_SH), which preserves concurrent reads.
// The lock is released when the underlying process exits, so a crashed holder
// never leaves a stale lock. A read on a virgin project (no lock file yet) runs
// unlocked: there is no writer state to race, and it must not materialize the
// project directory.
func (f *Filesystem) withLock(exclusive bool, fn func() error) error {
	how, flags := syscall.LOCK_SH, os.O_RDONLY
	if exclusive {
		how, flags = syscall.LOCK_EX, os.O_RDWR|os.O_CREATE
		if err := ensureDir(f.projectRoot()); err != nil {
			return err
		}
	}
	lf, err := os.OpenFile(f.lockPath(), flags, 0o600)
	if err != nil {
		if !exclusive && os.IsNotExist(err) {
			return fn() // virgin project: nothing to serialize against
		}
		return fmt.Errorf("store: open lock: %w", err)
	}
	defer func() { _ = lf.Close() }()
	if err := syscall.Flock(int(lf.Fd()), how); err != nil {
		return fmt.Errorf("store: flock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) }()
	return fn()
}

// Put implements TokenStore.
func (f *Filesystem) Put(sessionID, ticket string) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	return f.withLock(true, func() error {
		if err := ensureDir(f.ticketDir()); err != nil {
			return err
		}
		return writeAtomic(f.ticketPath(sessionID), []byte(ticket+"\n"))
	})
}

// Get implements TokenStore.
func (f *Filesystem) Get(sessionID string) (string, error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", err
	}
	var out string
	err := f.withLock(false, func() error {
		s, e := readTrimmed(f.ticketPath(sessionID))
		out = s
		return e
	})
	return out, err
}

// Delete implements TokenStore. Missing keys are not an error.
func (f *Filesystem) Delete(sessionID string) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	return f.withLock(true, func() error {
		return removeIfExists(f.ticketPath(sessionID))
	})
}

// Reset implements TokenStore: purge every ticket for this project. The
// session file is left untouched — callers manage session state separately.
func (f *Filesystem) Reset() error {
	return f.withLock(true, func() error {
		err := os.RemoveAll(f.ticketDir())
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: reset tickets: %w", err)
		}
		return nil
	})
}

// PutActive implements SessionStore.
func (f *Filesystem) PutActive(sessionID string) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	return f.withLock(true, func() error {
		if err := ensureDir(f.projectRoot()); err != nil {
			return err
		}
		return writeAtomic(f.sessionPath(), []byte(sessionID+"\n"))
	})
}

// GetActive implements SessionStore.
func (f *Filesystem) GetActive() (string, error) {
	var out string
	err := f.withLock(false, func() error {
		s, e := readTrimmed(f.sessionPath())
		out = s
		return e
	})
	return out, err
}

// ClearActive implements SessionStore. Idempotent.
func (f *Filesystem) ClearActive() error {
	return f.withLock(true, func() error {
		return removeIfExists(f.sessionPath())
	})
}

// validateSessionID rejects values that would escape the ticket dir or
// break filenames on common filesystems. Session ids in the wild are UUIDs
// from the agents; this is defense-in-depth on the API boundary.
func validateSessionID(sid string) error {
	if sid == "" {
		return errors.New("store: session id is empty")
	}
	if strings.ContainsAny(sid, "/\\\x00") {
		return fmt.Errorf("store: session id contains forbidden character: %q", sid)
	}
	return nil
}

// ensureDir mkdir-p's the given path with 0700 perms. Idempotent.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store: mkdir %s: %w", dir, err)
	}
	return nil
}

// writeAtomic writes content to path via a same-dir tmp file + rename.
// File mode is pinned to 0600 explicitly for portability.
func writeAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("store: create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("store: chmod %s: %w", tmpName, err)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("store: write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("store: sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("store: close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("store: rename %s -> %s: %w", tmpName, path, err)
	}
	return nil
}

// readTrimmed reads a file and returns its content stripped of trailing
// whitespace, or ErrNotFound when the file is absent.
func readTrimmed(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("store: read %s: %w", path, err)
	}
	return strings.TrimRight(string(raw), " \t\r\n"), nil
}

// removeIfExists deletes path, treating a missing file as success.
func removeIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: remove %s: %w", path, err)
	}
	return nil
}
