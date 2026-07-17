package translate

import (
	"reflect"
	"testing"
)

func TestGeneric(t *testing.T) {
	ok := Generic(0, "all good")
	if ok["status"] != "OK" || ok["exit_code"] != 0 || ok["raw_message"] != "all good" {
		t.Errorf("Generic(0) = %v", ok)
	}
	failed := Generic(2, "boom")
	if failed["status"] != "EXECUTION_FAILED" || failed["exit_code"] != 2 {
		t.Errorf("Generic(2) = %v", failed)
	}
}

func TestSyntaxError(t *testing.T) {
	out := SyntaxError(Generic(1, "raw"), "main.go:3: expected '}'")
	if out["error_category"] != "GO_SYNTAX_ERROR" {
		t.Errorf("error_category = %v", out["error_category"])
	}
	guidance, ok := out["remediation_guidance"].(map[string]any)
	if !ok {
		t.Fatal("remediation_guidance missing")
	}
	actions, ok := guidance["allowed_actions"].([]string)
	if !ok || len(actions) != 2 || actions[1] != "main.go:3: expected '}'" {
		t.Errorf("allowed_actions = %v, want the parser detail as second entry", guidance["allowed_actions"])
	}
}

func TestPolicyViolation(t *testing.T) {
	msgs := []string{"ADR-090: raw colors are forbidden"}
	out := PolicyViolation(Generic(1, ""), msgs)
	if out["error_category"] != "ARCHITECTURAL_INVARIANT_VIOLATION" {
		t.Errorf("error_category = %v", out["error_category"])
	}
	guidance := out["remediation_guidance"].(map[string]any)
	if !reflect.DeepEqual(guidance["violations"], msgs) {
		t.Errorf("violations = %v, want %v", guidance["violations"], msgs)
	}
}

func TestDBConflict(t *testing.T) {
	out := DBConflict(Generic(1, ""), "payments", "v2.4.1")
	if out["error_category"] != "DATABASE_SCHEMA_CONFLICT" {
		t.Errorf("error_category = %v", out["error_category"])
	}
	guidance := out["remediation_guidance"].(map[string]any)
	if guidance["context_update"] != "Current staging schema version: v2.4.1" {
		t.Errorf("context_update = %v", guidance["context_update"])
	}
}
