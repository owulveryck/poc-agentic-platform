package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCorpusFingerprintTracksChanges(t *testing.T) {
	dir := t.TempDir()
	empty := corpusFingerprint(dir, "")
	f := filepath.Join(dir, "ADR-999.rego")
	if err := os.WriteFile(f, []byte("package a"), 0o600); err != nil {
		t.Fatal(err)
	}
	added := corpusFingerprint(dir)
	if added == empty {
		t.Fatal("adding a file must change the fingerprint")
	}
	if corpusFingerprint(dir) != added {
		t.Fatal("an unchanged corpus must keep its fingerprint")
	}
	if err := os.WriteFile(f, []byte("package a\n\nx := 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if corpusFingerprint(dir) == added {
		t.Fatal("rewriting a file must change the fingerprint")
	}
	if corpusFingerprint(filepath.Join(dir, "absent")) != corpusFingerprint() {
		t.Fatal("a missing directory contributes nothing")
	}
}

func TestWatchCorpusSignalsOnceTheChangeSettles(t *testing.T) {
	dir := t.TempDir()
	changed := make(chan struct{}, 1)
	go watchCorpus(10*time.Millisecond, changed, dir)

	select {
	case <-changed:
		t.Fatal("no change on disk must not trigger a reload")
	case <-time.After(50 * time.Millisecond):
	}

	if err := os.WriteFile(filepath.Join(dir, "ADR-999.rego"), []byte("package a"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("a corpus change must trigger a reload")
	}

	select {
	case <-changed:
		t.Fatal("one change must trigger one reload")
	case <-time.After(50 * time.Millisecond):
	}
}
