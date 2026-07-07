// Package dbmigrate implements the apply_db_migration Smart Tool. The PoC
// simulates a staging database whose schema state is known to the platform —
// exactly the context an agent cannot guess and the reason semantic feedback
// amplifies it.
package dbmigrate

import (
	"strings"

	"github.com/owulveryck/poc-agentic-platform/internal/smarttools/translate"
)

// stagingTables simulates the platform's knowledge of the staging schema.
var stagingTables = map[string]bool{"payments": true, "users": true}

const stagingSchemaVersion = "v2.4.1"

// Tool is the apply_db_migration Smart Tool.
type Tool struct{}

// ID implements smarttools.Tool.
func (Tool) ID() string { return "apply_db_migration" }

// Run applies the migration against the simulated staging state.
// payload: {"statement": "CREATE TABLE payments (...)"}
//
// Returns {"status": "OK", "applied": targets} on success, or a
// translate.DBConflict payload when the target table already exists in staging.
func (Tool) Run(targets []string, payload map[string]any) map[string]any {
	stmt, _ := payload["statement"].(string)
	lower := strings.ToLower(stmt)
	if strings.HasPrefix(lower, "create table ") && !strings.Contains(lower, "if not exists") {
		fields := strings.Fields(lower)
		table := strings.Trim(fields[2], "(")
		if stagingTables[table] {
			base := translate.Generic(1, `SQLSTATE 42P07: table "`+table+`" already exists`)
			return translate.DBConflict(base, table, stagingSchemaVersion)
		}
	}
	return map[string]any{"status": "OK", "applied": targets}
}
