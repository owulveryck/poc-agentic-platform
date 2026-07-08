package patchcode

import "testing"

func TestEmptyTargetsDoesNotPanic(t *testing.T) {
	out := Tool{}.Run(nil, map[string]any{"content": "package main"})
	if out["status"] != "EXECUTION_FAILED" {
		t.Fatalf("expected EXECUTION_FAILED for empty targets, got %v", out)
	}
}

func TestValidGoContentPasses(t *testing.T) {
	out := Tool{}.Run([]string{"main.go"}, map[string]any{"content": "package main\n\nfunc main() {}\n"})
	if out["status"] != "OK" {
		t.Fatalf("expected OK, got %v", out)
	}
}

func TestInvalidGoContentReturnsSyntaxError(t *testing.T) {
	out := Tool{}.Run([]string{"main.go"}, map[string]any{"content": "package main\n\nfunc {"})
	if out["error_category"] != "GO_SYNTAX_ERROR" {
		t.Fatalf("expected GO_SYNTAX_ERROR, got %v", out)
	}
}
