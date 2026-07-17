package ppg.linter

import rego.v1

# ADR-100: capability tickets and session ids MUST be persisted through
# the internal/store package. A plan whose steps touch a file under
# adapters/ AND mention the legacy bare-file names as string literals
# indicates a regression to cwd-scoped state — reject at lock time.

violation contains v if {
	input.view == "plan"
	some step in input.steps
	target_is_adapter_go_file(step)
	step_reintroduces_legacy_state_file(step)
	v := {
		"policy_id": "per_machine_state_directory",
		"message":   "State-persistence invariant: this plan step touches an adapter Go file and mentions '.ppg-ticket' or '.ppg-session' in its action — those bare-file names are the pre-ADR-100 protocol. Persist capability tickets and session ids through internal/store (TokenStore, SessionStore) instead.",
		"nature":    "amplifier",
	}
}

target_is_adapter_go_file(step) if {
	some t in step.targets
	startswith(t, "adapters/")
	endswith(t, ".go")
}

step_reintroduces_legacy_state_file(step) if {
	contains(step.action, ".ppg-ticket")
}

step_reintroduces_legacy_state_file(step) if {
	contains(step.action, ".ppg-session")
}
