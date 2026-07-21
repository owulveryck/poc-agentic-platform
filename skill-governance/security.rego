package ppg.skills.governance

import rego.v1

# A skill is tier >= 1 (privileged) when it instructs file modifications
# (Edit/Write, tier 1) or shell execution (Bash, tier 2). Such skills MUST
# include a companion SKILL.rego that declares the additional plan
# governance requirements they impose. This ensures the plan linter
# (PPG /lock_in_plan) can enforce those requirements at runtime, closing
# the loop between skill authoring and plan execution.
#
# The tier itself is computed by the Go linter — the single source of tier
# truth (internal/skill.Linter.Tier) — and handed to this policy as
# input.tier. Consuming it here instead of re-deriving it from body
# keywords means the Go and Rego views of "privileged" cannot drift.

privileged if input.tier >= 1

violation contains v if {
	privileged
	not input.rego_policy
	v := {
		"field":   "rego_policy",
		"message": "Skills that instruct file modifications (tier ≥ 1) must include a companion SKILL.rego declaring their plan governance requirements.",
		"nature":  "amplifier",
	}
}
