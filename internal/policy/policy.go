// Package policy is the shared OPA/Rego evaluation core behind every
// deterministic gate in the platform. It is deliberately domain-agnostic: it
// compiles a query over a set of .rego files and evaluates it against an
// arbitrary input document, decoding the resulting rule set into a caller-chosen
// violation type.
//
// The same engine powers three altitudes of enforcement — the plan linter
// (plan view), the in-tool/guard artifact check (artifact view), and the
// apply-time diff gate (changeset view) — as well as skill governance, each
// with its own query, input document, and violation shape. Extracting it here
// removes the near-verbatim evaluation loop that internal/linter and
// internal/skill used to duplicate.
package policy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Evaluator wraps a compiled OPA query. A nil or empty Evaluator is a valid
// no-op that yields no violations, so callers with no policy files still work.
type Evaluator struct {
	prepared *rego.PreparedEvalQuery
}

// Prepare compiles query over the given .rego files. With no files it returns a
// ready no-op Evaluator (Eval yields nothing), matching the "declarative-only,
// no rego" case where a policy corpus may be empty.
func Prepare(query string, regoPaths []string) (*Evaluator, error) {
	if len(regoPaths) == 0 {
		return &Evaluator{}, nil
	}
	ctx := context.Background()
	pq, err := rego.New(
		rego.Query(query),
		rego.Load(regoPaths, nil),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("preparing OPA query %q: %w", query, err)
	}
	return &Evaluator{prepared: &pq}, nil
}

// Ready reports whether the evaluator has a compiled query. A not-ready
// evaluator evaluates to no violations.
func (e *Evaluator) Ready() bool { return e != nil && e.prepared != nil }

// Eval evaluates the prepared query against input and decodes the violation set
// into []T. It fails closed: any evaluation or decode failure is returned as an
// error so the caller rejects rather than silently passing. A not-ready
// evaluator returns (nil, nil).
func Eval[T any](e *Evaluator, input any) ([]T, error) {
	if !e.Ready() {
		return nil, nil
	}
	ctx := context.Background()
	rs, err := e.prepared.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("evaluating policy: %w", err)
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 || rs[0].Expressions[0].Value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(rs[0].Expressions[0].Value)
	if err != nil {
		return nil, fmt.Errorf("encoding policy result: %w", err)
	}
	var out []T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding policy violations: %w", err)
	}
	return out, nil
}
