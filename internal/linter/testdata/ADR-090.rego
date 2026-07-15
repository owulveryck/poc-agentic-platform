package ppg.linter

import rego.v1

# ADR-090: any plan that touches UI files must include a step that reads
# the design tokens — the model must acknowledge the tokens exist before
# planning visual changes.

violation contains v if {
	some step in input.steps
	target_is_ui(step)
	not plan_reads_design_tokens
	v := {
		"policy_id": "design_tokens_referenced",
		"message":   "Design-system invariant: this plan touches a UI file but no step reads design/tokens.css. Add a step whose targets include \"design/tokens.css\" (Read is enough) so the model plans against the canonical palette.",
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
