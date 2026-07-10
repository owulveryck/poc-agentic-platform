package ppg.skills.add_payment_method

import rego.v1

# Companion policy of the add-payment-method skill (dual-representation
# artifact, validated at POST /validate_skill). Plans built by this skill
# add a payment provider: they must create the schema migration before the
# code that uses it. The rule mirrors the shape of adr/ADR-051.rego; when
# the plan linter learns to load skill companions (Gate 3), it will union
# with the ADR policies automatically.

violation contains v if {
	some step in input.steps
	endswith(step.targets[_], ".go")
	not plan_has_migration
	v := {
		"policy_id": "payment_provider_migration_first",
		"message":   "A payment provider needs its schema migration: add a step targeting a file under migrations/ before the code steps.",
		"nature":    "amplifier",
	}
}

plan_has_migration if {
	some step in input.steps
	startswith(step.targets[_], "migrations/")
}
