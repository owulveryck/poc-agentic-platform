package ppg.linter

import rego.v1

violation contains v if {
	input.repository_context.tech_stack[_] == "Go"
	not plan_has_go_test
	v := {
		"policy_id": "go_tests_present",
		"message":   "SDLC invariant violated: the plan must contain a 'go test' step for a Go stack.",
		"nature":    "amplifier",
	}
}

plan_has_go_test if {
	input.steps[_].tool == "go-test"
}
