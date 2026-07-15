package ppg.skills.design_system

import rego.v1

# Companion of ADR-090. Same shape, scoped to this skill's registry
# submission: plans produced under the design-system workflow must
# include a step reading design/tokens.css whenever they touch UI files.
# Once the skill-companion policies are unioned into the plan linter
# (Gate 3, tracked in the platform TODO), this rule also runs at
# lock_in_plan time for every session that invoked /design-system.

violation contains v if {
	some step in input.steps
	target_is_ui(step)
	not plan_reads_design_tokens
	v := {
		"policy_id": "design_tokens_referenced",
		"message":   "Design-system skill: this plan touches a UI file but no step reads design/tokens.css. Add a step whose targets include \"design/tokens.css\" so the model plans against the canonical palette.",
		"nature":    "amplifier",
	}
}

target_is_ui(step) if {
	endswith(step.targets[_], ".html")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".css")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".tsx")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".jsx")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".svelte")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".vue")
}

plan_reads_design_tokens if {
	some step in input.steps
	step.targets[_] == "design/tokens.css"
}
