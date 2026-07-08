package ppg.linter

import rego.v1

frozen_paths := {"internal/old_payment.go", "internal/auth/"}

violation contains v if {
	some step in input.steps
	some target in step.targets
	some fp in frozen_paths
	startswith(target, fp)
	v := {
		"policy_id": "explicit_frozen_files_enumeration",
		"message":   concat("", ["Frozen zone: modifying '", target, "' is forbidden (deprecated legacy code)."]),
		"nature":    "compensatory",
	}
}
