package policy

import "testing"

type finding struct {
	ID string `json:"id"`
}

func TestEvalDiscriminatesByInput(t *testing.T) {
	e, err := Prepare("data.ppg.test.violation", []string{"testdata/sample.rego"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if !e.Ready() {
		t.Fatal("evaluator should be ready with a loaded policy")
	}

	a, err := Eval[finding](e, map[string]string{"view": "a"})
	if err != nil {
		t.Fatalf("Eval a: %v", err)
	}
	if len(a) != 1 || a[0].ID != "rule-a" {
		t.Fatalf("view a: want [rule-a], got %v", a)
	}

	b, err := Eval[finding](e, map[string]string{"view": "b"})
	if err != nil {
		t.Fatalf("Eval b: %v", err)
	}
	if len(b) != 1 || b[0].ID != "rule-b" {
		t.Fatalf("view b: want [rule-b], got %v", b)
	}

	none, err := Eval[finding](e, map[string]string{"view": "c"})
	if err != nil {
		t.Fatalf("Eval c: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("view c: want no findings, got %v", none)
	}
}

func TestEmptyEvaluatorIsNoOp(t *testing.T) {
	e, err := Prepare("data.ppg.test.violation", nil)
	if err != nil {
		t.Fatalf("Prepare with no files: %v", err)
	}
	if e.Ready() {
		t.Fatal("evaluator with no files should not be ready")
	}
	out, err := Eval[finding](e, map[string]string{"view": "a"})
	if err != nil {
		t.Fatalf("Eval on empty: %v", err)
	}
	if out != nil {
		t.Fatalf("empty evaluator should yield nil, got %v", out)
	}
}

func TestNondeterministicBuiltinsRejectedAtCompileTime(t *testing.T) {
	// A policy calling http.send (or any built-in OPA marks nondeterministic)
	// must fail at Prepare time: determinism holds by construction.
	src := `package ppg.skills.evil

import rego.v1

violation contains v if {
	resp := http.send({"method": "GET", "url": "http://example.com"})
	resp.status_code == 200
	v := {"policy_id": "evil", "message": "x", "nature": "amplifier"}
}
`
	if _, err := PrepareModule("data.ppg.skills.evil.violation", "evil.rego", src); err == nil {
		t.Fatal("PrepareModule accepted a policy calling http.send; nondeterministic built-ins must be rejected at compile time")
	}
}
