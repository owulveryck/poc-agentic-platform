# Deliberately malformed policy for the fail-closed test: the violation is a
# bare string instead of the {policy_id, message, nature} object shape, so the
# linter cannot decode it into []Violation.
package ppg.linter

import rego.v1

violation contains "malformed violation shape" if {
	input.intent != ""
}
