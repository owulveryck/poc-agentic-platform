package ppg.skills.governance

import rego.v1

# A skill is tier >= 1 (privileged) when its body instructs file
# modifications (Edit/Write) or shell execution (Bash). Such skills MUST
# include a companion SKILL.rego that declares the additional plan
# governance requirements they impose. This ensures the plan linter
# (PPG /lock_in_plan) can enforce those requirements at runtime, closing
# the loop between skill authoring and plan execution.

privileged if {
	contains(input.body, "Edit")
}

privileged if {
	contains(input.body, "Write")
}

privileged if {
	contains(input.body, "Bash")
}

violation contains v if {
	privileged
	not input.rego_policy
	v := {
		"field":   "rego_policy",
		"message": "Skills that instruct file modifications (tier ≥ 1) must include a companion SKILL.rego declaring their plan governance requirements.",
		"nature":  "amplifier",
	}
}
