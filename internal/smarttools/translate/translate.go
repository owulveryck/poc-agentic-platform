// Package translate holds the two result translators of a Smart Tool — kept
// deliberately separate because they live on opposite sides of the
// durability axis:
//
//   - Generic (compensatory): turns a raw exit code / stack trace into minimal
//     JSON. Scheduled to disappear once models read raw traces reliably.
//   - Semantic enrichers (amplifier): add the business context the model
//     cannot guess (staging state, interface definition, violated ADR).
//
// The day the generic translator sunsets, the enrichers stay untouched.
package translate

// GenericNature tags the raw→JSON translator as compensatory on the
// durability axis. It is used in the transition-debt report.
const GenericNature = "compensatory"

// GenericSunsetCondition is the measurable condition under which the Generic
// translator can be removed. It is referenced directly by the debt report so
// that the sunset condition is defined in one place.
const GenericSunsetCondition = "Model reads raw stack traces reliably on >95% of an internal benchmark."

// Generic converts a raw execution outcome into a minimal structured payload.
func Generic(exitCode int, raw string) map[string]any {
	status := "OK"
	if exitCode != 0 {
		status = "EXECUTION_FAILED"
	}
	return map[string]any{
		"status":      status,
		"exit_code":   exitCode,
		"raw_message": raw,
	}
}

// SyntaxError enriches a Go parse failure with actionable guidance.
func SyntaxError(base map[string]any, detail string) map[string]any {
	base["error_category"] = "GO_SYNTAX_ERROR"
	base["message"] = "The patched file does not parse as valid Go."
	base["remediation_guidance"] = map[string]any{
		"allowed_actions": []string{
			"Fix the syntax error reported below and resubmit the patch.",
			detail,
		},
	}
	return base
}

// DBConflict enriches a schema conflict with the current staging state — the
// context the model cannot guess on its own.
func DBConflict(base map[string]any, table, schemaVersion string) map[string]any {
	base["error_category"] = "DATABASE_SCHEMA_CONFLICT"
	base["message"] = "Table '" + table + "' already exists in staging."
	base["remediation_guidance"] = map[string]any{
		"allowed_actions": []string{
			"Use 'get_db_schema' to inspect the current structure.",
			"Add an 'IF NOT EXISTS' clause or rename the table.",
		},
		"context_update": "Current staging schema version: " + schemaVersion,
	}
	return base
}
