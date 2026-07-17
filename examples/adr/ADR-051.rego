package ppg.linter

import rego.v1

violation contains v if {
	input.view == "plan"
	some step in input.steps
	target_is_db(step)
	not plan_has_migration
	v := {
		"policy_id": "db_migration_precedes_code",
		"message":   "Invalid ordering: a schema migration must accompany any database change. Add a step whose tool is \"db-migration-generator\", or a step targeting a file under migrations/.",
		"nature":    "amplifier",
	}
}

target_is_db(step) if {
	endswith(step.targets[_], ".sql")
}

target_is_db(step) if {
	contains(step.targets[_], "db/")
}

plan_has_migration if {
	input.steps[_].tool == "db-migration-generator"
}

# Robustness: agents often encode the migration as a file creation under
# migrations/ with their own tool names. Accept that encoding too.
plan_has_migration if {
	some step in input.steps
	startswith(step.targets[_], "migrations/")
}
