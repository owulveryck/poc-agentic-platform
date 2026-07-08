package dbmigrate

import "testing"

func TestTruncatedCreateTableDoesNotPanic(t *testing.T) {
	out := Tool{}.Run(nil, map[string]any{"statement": "create table "})
	if out["status"] != "EXECUTION_FAILED" {
		t.Fatalf("expected EXECUTION_FAILED for a truncated statement, got %v", out)
	}
}

func TestExistingTableReturnsConflict(t *testing.T) {
	out := Tool{}.Run([]string{"payments"}, map[string]any{"statement": "CREATE TABLE payments (id int)"})
	if out["error_category"] != "DATABASE_SCHEMA_CONFLICT" {
		t.Fatalf("expected DATABASE_SCHEMA_CONFLICT, got %v", out)
	}
}

func TestNewTablePasses(t *testing.T) {
	out := Tool{}.Run([]string{"refunds"}, map[string]any{"statement": "CREATE TABLE refunds (id int)"})
	if out["status"] != "OK" {
		t.Fatalf("expected OK, got %v", out)
	}
}
