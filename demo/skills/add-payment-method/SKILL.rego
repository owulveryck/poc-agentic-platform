package ppg.skills.add_payment_method

import rego.v1

# Companion policy of the add-payment-method skill (dual-representation
# artifact, validated at POST /validate_skill). Plans built by this skill
# add a payment provider: they must create the schema migration before the
# code that uses it. The rule mirrors the shape of examples/adr/ADR-051.rego; when
# the plan linter loads skill companions (Gate 3, since v1.0.0 via
# ppg -skills), it unions with the ADR policies automatically for every
# plan that declares skill_id "add-payment-method".
#
# ENFORCEMENT SCOPE — read before relying on this rule.
# This is a PLAN-VIEW invariant: it reasons about the ORDER of plan steps
# (a migrations/ step before the .go steps), which is information that only
# exists at plan altitude. The linter evaluates a skill's plan-view rules
# only for a plan that DECLARES this skill_id (see internal/linter/linter.go
# Validate / evaluateSkillCompanion). Unlike design-system's raw-color ban,
# an ordering invariant CANNOT be re-expressed as a content-view (artifact/
# changeset) rule — a single edited file's bytes carry no evidence about
# whether a migration step exists — so there is no content-view backstop to
# union in when skill_id is omitted. Consequence: a plan that does not
# declare skill_id "add-payment-method" bypasses this ordering check. Bind
# the skill to payment intents (or make skill_id mandatory for payment work)
# if this invariant must hold unconditionally; the union-semantics guarantee
# covers content rules only.

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
