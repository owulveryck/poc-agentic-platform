package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRoot_XDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	got, err := DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	if got != "/tmp/xdg/ppg" {
		t.Errorf("DefaultRoot = %q, want /tmp/xdg/ppg", got)
	}
}

func TestDefaultRoot_HomeFallback(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	// os.UserHomeDir uses HOME on unix; force a known value.
	t.Setenv("HOME", "/tmp/fakehome")
	got, err := DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	want := filepath.Join("/tmp/fakehome", ".local", "state", "ppg")
	if got != want {
		t.Errorf("DefaultRoot = %q, want %q", got, want)
	}
}

func TestResolveRoot_FlagWins(t *testing.T) {
	t.Setenv(EnvStoreRoot, "/env/root")
	t.Setenv("XDG_STATE_HOME", "/xdg")
	got, err := ResolveRoot("/flag/root")
	if err != nil {
		t.Fatal(err)
	}
	// Normalize resolves symlinks but /flag/root does not exist, so
	// EvalSymlinks returns ErrNotExist and Normalize falls back to Clean(Abs).
	want, _ := Normalize("/flag/root")
	if got != want {
		t.Errorf("ResolveRoot(flag) = %q, want %q", got, want)
	}
}

func TestResolveRoot_EnvWhenNoFlag(t *testing.T) {
	t.Setenv(EnvStoreRoot, "/env/root")
	t.Setenv("XDG_STATE_HOME", "/xdg")
	got, err := ResolveRoot("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Normalize("/env/root")
	if got != want {
		t.Errorf("ResolveRoot(env) = %q, want %q", got, want)
	}
}

func TestResolveRoot_DefaultWhenNoneSet(t *testing.T) {
	t.Setenv(EnvStoreRoot, "")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-defaults")
	got, err := ResolveRoot("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Normalize("/tmp/xdg-defaults/ppg")
	if got != want {
		t.Errorf("ResolveRoot(default) = %q, want %q", got, want)
	}
}

func TestResolveProjectDir_FlagWins(t *testing.T) {
	t.Setenv(EnvProjectDir, "/env/proj")
	got, err := ResolveProjectDir("/flag/proj", "/fallback")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Normalize("/flag/proj")
	if got != want {
		t.Errorf("ResolveProjectDir(flag) = %q, want %q", got, want)
	}
}

func TestResolveProjectDir_EnvWhenNoFlag(t *testing.T) {
	t.Setenv(EnvProjectDir, "/env/proj")
	got, err := ResolveProjectDir("", "/fallback")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Normalize("/env/proj")
	if got != want {
		t.Errorf("ResolveProjectDir(env) = %q, want %q", got, want)
	}
}

func TestResolveProjectDir_FallbackWhenNoneSet(t *testing.T) {
	t.Setenv(EnvProjectDir, "")
	got, err := ResolveProjectDir("", "/fallback/proj")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Normalize("/fallback/proj")
	if got != want {
		t.Errorf("ResolveProjectDir(fallback) = %q, want %q", got, want)
	}
}

func TestResolveProjectDir_RequiredErrorsWhenBlank(t *testing.T) {
	t.Setenv(EnvProjectDir, "")
	_, err := ResolveProjectDir("", "")
	if err == nil {
		t.Fatal("ResolveProjectDir with no source should error")
	}
	if !strings.Contains(err.Error(), "PPG_PROJECT_DIR") {
		t.Errorf("error message should mention PPG_PROJECT_DIR, got %q", err)
	}
}

func TestNormalize_ExistingSymlinkResolved(t *testing.T) {
	realDir := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	got, err := Normalize(link)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Normalize(realDir)
	if got != want {
		t.Errorf("Normalize(link) = %q, want %q", got, want)
	}
}

func TestNormalize_NonExistentReturnsClean(t *testing.T) {
	got, err := Normalize("/does/not/exist/anywhere")
	if err != nil {
		t.Fatalf("Normalize on non-existent: %v", err)
	}
	if got != "/does/not/exist/anywhere" {
		t.Errorf("Normalize = %q, want /does/not/exist/anywhere", got)
	}
}
