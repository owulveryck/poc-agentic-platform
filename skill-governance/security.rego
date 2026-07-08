package ppg.skills.governance

import rego.v1

# A skill instructs file modifications when its body mentions Edit or Write.
# Such skills are classified as tier ≥ 1 and MUST include a companion SKILL.rego
# that declares the additional plan governance requirements they impose.
# This ensures the plan linter (PPG /lock_in_plan) can enforce those requirements
# at runtime, closing the loop between skill authoring and plan execution.

modifies_files if {
	contains(input.body, "Edit")
}

modifies_files if {
	contains(input.body, "Write")
}

violation contains v if {
	modifies_files
	not input.rego_policy
	v := {
		"field":   "rego_policy",
		"message": "Skills that instruct file modifications (tier ≥ 1) must include a companion SKILL.rego declaring their plan governance requirements.",
		"nature":  "amplifier",
	}
}
