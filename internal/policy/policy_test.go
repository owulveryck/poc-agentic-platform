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
