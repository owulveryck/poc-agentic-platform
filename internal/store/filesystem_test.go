package store

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newFS(t *testing.T) (*Filesystem, string) {
	t.Helper()
	root := t.TempDir()
	projectDir := t.TempDir()
	fs, err := NewFilesystem(root, projectDir)
	if err != nil {
		t.Fatalf("NewFilesystem: %v", err)
	}
	return fs, projectDir
}

func TestFilesystem_PutGetActive(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.PutActive("sess-1"); err != nil {
		t.Fatalf("PutActive: %v", err)
	}
	got, err := fs.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got != "sess-1" {
		t.Errorf("GetActive = %q, want sess-1", got)
	}
}

func TestFilesystem_GetActiveNotFound(t *testing.T) {
	fs, _ := newFS(t)
	_, err := fs.GetActive()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetActive on empty store: err = %v, want ErrNotFound", err)
	}
}

func TestFilesystem_ClearActiveIdempotent(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.ClearActive(); err != nil {
		t.Fatalf("ClearActive on empty store: %v", err)
	}
	if err := fs.PutActive("s"); err != nil {
		t.Fatal(err)
	}
	if err := fs.ClearActive(); err != nil {
		t.Fatalf("ClearActive: %v", err)
	}
	if _, err := fs.GetActive(); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetActive after clear: err = %v, want ErrNotFound", err)
	}
}

func TestFilesystem_PutGetTicket(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.Put("sid-1", "jwt-payload"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := fs.Get("sid-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "jwt-payload" {
		t.Errorf("Get = %q, want jwt-payload", got)
	}
}

func TestFilesystem_GetTicketNotFound(t *testing.T) {
	fs, _ := newFS(t)
	_, err := fs.Get("sid-1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on empty store: err = %v, want ErrNotFound", err)
	}
}

func TestFilesystem_DeleteIdempotent(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.Delete("sid-x"); err != nil {
		t.Fatalf("Delete on missing key: %v", err)
	}
	if err := fs.Put("sid-x", "t"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete("sid-x"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := fs.Get("sid-x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: err = %v, want ErrNotFound", err)
	}
}

func TestFilesystem_ResetPurgesTicketsButKeepsSession(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.PutActive("sess-A"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Put("sid-1", "t1"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Put("sid-2", "t2"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := fs.Get("sid-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(sid-1) after Reset: err = %v, want ErrNotFound", err)
	}
	if _, err := fs.Get("sid-2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(sid-2) after Reset: err = %v, want ErrNotFound", err)
	}
	got, err := fs.GetActive()
	if err != nil {
		t.Fatalf("GetActive after Reset: %v", err)
	}
	if got != "sess-A" {
		t.Errorf("GetActive after Reset = %q, want sess-A", got)
	}
}

func TestFilesystem_ResetIdempotent(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.Reset(); err != nil {
		t.Fatalf("Reset on empty store: %v", err)
	}
}

func TestFilesystem_RejectsSessionIDWithSlash(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.Put("a/b", "t"); err == nil {
		t.Fatal("Put with '/' should error")
	}
	if _, err := fs.Get("a/b"); err == nil {
		t.Fatal("Get with '/' should error")
	}
	if err := fs.Delete("a/b"); err == nil {
		t.Fatal("Delete with '/' should error")
	}
}

func TestFilesystem_RejectsEmptySessionID(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.Put("", "t"); err == nil {
		t.Fatal("Put with empty session id should error")
	}
	if err := fs.PutActive(""); err == nil {
		t.Fatal("PutActive with empty session id should error")
	}
}

func TestFilesystem_FilePermissionsAre0600And0700(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.PutActive("sess"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Put("sid", "t"); err != nil {
		t.Fatal(err)
	}
	assertMode := func(t *testing.T, path string, want os.FileMode) {
		t.Helper()
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := st.Mode().Perm(); got != want {
			t.Errorf("%s: mode = %v, want %v", path, got, want)
		}
	}
	assertMode(t, fs.sessionPath(), 0o600)
	assertMode(t, fs.ticketPath("sid"), 0o600)
	assertMode(t, fs.projectRoot(), 0o700)
	assertMode(t, fs.ticketDir(), 0o700)
}

func TestFilesystem_TrimsTrailingWhitespaceOnRead(t *testing.T) {
	fs, _ := newFS(t)
	if err := fs.PutActive("sess-42"); err != nil {
		t.Fatal(err)
	}
	// PutActive appends a newline; GetActive must strip it.
	raw, err := os.ReadFile(fs.sessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("session file should end with newline, got %q", raw)
	}
	got, err := fs.GetActive()
	if err != nil {
		t.Fatal(err)
	}
	if got != "sess-42" {
		t.Errorf("GetActive = %q, want sess-42 (trimmed)", got)
	}
}

func TestFilesystem_TwoProjectsDoNotCollide(t *testing.T) {
	root := t.TempDir()
	p1 := t.TempDir()
	p2 := t.TempDir()
	fs1, err := NewFilesystem(root, p1)
	if err != nil {
		t.Fatal(err)
	}
	fs2, err := NewFilesystem(root, p2)
	if err != nil {
		t.Fatal(err)
	}
	if fs1.slug == fs2.slug {
		t.Fatalf("distinct projects produced identical slugs: %s", fs1.slug)
	}
	if err := fs1.PutActive("s1"); err != nil {
		t.Fatal(err)
	}
	if err := fs2.PutActive("s2"); err != nil {
		t.Fatal(err)
	}
	got1, _ := fs1.GetActive()
	got2, _ := fs2.GetActive()
	if got1 != "s1" || got2 != "s2" {
		t.Errorf("cross-project contamination: %q / %q", got1, got2)
	}
}

func TestFilesystem_SlugIsBase64OfProjectDir(t *testing.T) {
	fs, projectDir := newFS(t)
	// The stored projectDir is normalized (EvalSymlinks-resolved), so we
	// re-encode from the store's own view to compare — this test asserts
	// the slug is a reversible encoding, not the raw temp path.
	want := base64.RawURLEncoding.EncodeToString([]byte(fs.ProjectDir()))
	if fs.slug != want {
		t.Errorf("slug = %s, want %s", fs.slug, want)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(fs.slug)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != fs.ProjectDir() {
		t.Errorf("slug decoded = %q, want %q", decoded, fs.ProjectDir())
	}
	// projectDir is the raw input; ProjectDir() is normalized.
	if !filepath.IsAbs(projectDir) {
		t.Errorf("test setup expected abs project dir, got %q", projectDir)
	}
}

func TestFilesystem_SymlinkNormalization(t *testing.T) {
	root := t.TempDir()
	realDir := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "linked")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	fs1, err := NewFilesystem(root, realDir)
	if err != nil {
		t.Fatal(err)
	}
	fs2, err := NewFilesystem(root, link)
	if err != nil {
		t.Fatal(err)
	}
	if fs1.slug != fs2.slug {
		t.Errorf("real and symlinked project dirs produced different slugs: %s vs %s",
			fs1.slug, fs2.slug)
	}
}

func TestNewFilesystem_RejectsEmpty(t *testing.T) {
	if _, err := NewFilesystem("", "/tmp/x"); err == nil {
		t.Error("empty root should error")
	}
	if _, err := NewFilesystem("/tmp/x", ""); err == nil {
		t.Error("empty project dir should error")
	}
}

func TestFilesystem_ConcurrentAccessIsSafe(t *testing.T) {
	fs, _ := newFS(t)
	// Two fixed session ids; goroutines hammer Put/Get/Delete/Reset/PutActive
	// concurrently. The advisory lock must serialize them: no data race
	// (checked under -race), no error, and no torn/partial reads.
	const workers = 40
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			sid := "11111111-1111-1111-1111-111111111111"
			switch i % 5 {
			case 0:
				if err := fs.Put(sid, "jwt-value"); err != nil {
					t.Errorf("Put: %v", err)
				}
			case 1:
				if _, err := fs.Get(sid); err != nil && !errors.Is(err, ErrNotFound) {
					t.Errorf("Get: %v", err)
				}
			case 2:
				if err := fs.Delete(sid); err != nil {
					t.Errorf("Delete: %v", err)
				}
			case 3:
				if err := fs.Reset(); err != nil {
					t.Errorf("Reset: %v", err)
				}
			case 4:
				if err := fs.PutActive("22222222-2222-2222-2222-222222222222"); err != nil {
					t.Errorf("PutActive: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestFilesystem_ResetDoesNotRaceLoseTicket(t *testing.T) {
	// A Put concurrent with a Reset must resolve deterministically (the lock
	// makes them atomic w.r.t. each other): after both complete, Get returns
	// either the ticket or ErrNotFound — never a torn value or an error.
	fs, _ := newFS(t)
	sid := "11111111-1111-1111-1111-111111111111"
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = fs.Put(sid, "jwt-value") }()
	go func() { defer wg.Done(); _ = fs.Reset() }()
	wg.Wait()
	got, err := fs.Get(sid)
	if err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Put||Reset: unexpected error %v", err)
	}
	if err == nil && got != "jwt-value" {
		t.Fatalf("torn read: got %q", got)
	}
}
