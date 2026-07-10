package ppg.linter

import rego.v1

violation contains v if {
	input.repository_context.tech_stack[_] == "Go"
	not plan_has_go_test
	v := {
		"policy_id": "go_tests_present",
		"message":   "SDLC invariant violated: the plan has no test step. Add a step whose tool is \"go-test\", or whose action runs 'go test'.",
		"nature":    "amplifier",
	}
}

plan_has_go_test if {
	input.steps[_].tool == "go-test"
}

# Robustness: coding agents encode steps with their own tool names
# (e.g. tool "Bash" with action "go test ./..."). Accept that encoding too.
plan_has_go_test if {
	some step in input.steps
	contains(lower(step.action), "go test")
}
