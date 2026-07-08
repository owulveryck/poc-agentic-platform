package ppg.linter

import rego.v1

violation contains v if {
	some step in input.steps
	target_is_db(step)
	not plan_has_migration
	v := {
		"policy_id": "db_migration_precedes_code",
		"message":   "Invalid ordering: a schema migration step (tool 'db-migration-generator') must accompany any database change.",
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
