package linter

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The ADR-*.rego files under testdata/ are copies of the canonical policies
// in examples/adr, kept as regular files so the test corpus stays
// self-contained. This test pins each copy to its source: editing one side
// without the other fails here instead of drifting silently. testdata/ may
// hold a subset of examples/adr (BAD-001.rego is a test-only fixture and is
// not checked).
func TestTestdataRegoMatchesExamples(t *testing.T) {
	copies, err := filepath.Glob(filepath.Join("testdata", "ADR-*.rego"))
	if err != nil {
		t.Fatalf("glob testdata: %v", err)
	}
	if len(copies) == 0 {
		t.Fatal("no ADR-*.rego files under testdata/ — glob broken?")
	}
	for _, copyPath := range copies {
		name := filepath.Base(copyPath)
		canonicalPath := filepath.Join("..", "..", "examples", "adr", name)
		got, err := os.ReadFile(copyPath)
		if err != nil {
			t.Fatalf("read %s: %v", copyPath, err)
		}
		want, err := os.ReadFile(canonicalPath)
		if err != nil {
			t.Fatalf("%s has no canonical counterpart at %s: %v", name, canonicalPath, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from %s — update both sides together", copyPath, canonicalPath)
		}
	}
}
