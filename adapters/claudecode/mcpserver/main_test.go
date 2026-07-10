package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionIDFromFile(t *testing.T) {
	dir := t.TempDir()
	if got := sessionIDFromFile(dir); got != "" {
		t.Fatalf("no session file should yield an empty id, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionFile), []byte("sess-42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := sessionIDFromFile(dir); got != "sess-42" {
		t.Fatalf("session id should be read and trimmed, got %q", got)
	}
}
