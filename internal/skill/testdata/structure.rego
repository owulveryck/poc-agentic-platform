package ppg.skills.governance

import rego.v1

# name is required
violation contains v if {
	not input.name
	v := {
		"field":   "name",
		"message": "name is required",
		"nature":  "amplifier",
	}
}

# name must be lowercase-kebab-case, at most 32 characters
violation contains v if {
	input.name
	not regex.match(`^[a-z][a-z0-9-]{0,31}$`, input.name)
	v := {
		"field":   "name",
		"message": "name must be lowercase-kebab-case and at most 32 characters",
		"nature":  "amplifier",
	}
}

# description is required
violation contains v if {
	not input.description
	v := {
		"field":   "description",
		"message": "description is required",
		"nature":  "amplifier",
	}
}

# description too short — not discoverable
violation contains v if {
	input.description
	count(input.description) < 50
	v := {
		"field":   "description",
		"message": "description must be at least 50 characters to be discoverable",
		"nature":  "amplifier",
	}
}

# description too long — degrades context window efficiency
violation contains v if {
	input.description
	count(input.description) > 500
	v := {
		"field":   "description",
		"message": "description must be at most 500 characters",
		"nature":  "amplifier",
	}
}

# version is required for registry publication
violation contains v if {
	not input.version
	v := {
		"field":   "version",
		"message": "version is required for registry publication (semver, e.g. '1.0.0')",
		"nature":  "amplifier",
	}
}

# argument-hint is required when the body uses $ARGUMENTS
violation contains v if {
	contains(input.body, "$ARGUMENTS")
	not input.argument_hint
	v := {
		"field":   "argument_hint",
		"message": "argument-hint is required when the skill body uses $ARGUMENTS",
		"nature":  "amplifier",
	}
}

# description must start with a verb — third-person form, e.g. "Adds", "Runs".
# Deliberately naive (a capitalized word ending in s), same assumed posture as
# the tier keywords: deterministic and cheap beats clever.
violation contains v if {
	input.description
	not regex.match(`^[A-Z][a-z]+s\s`, input.description)
	v := {
		"field":   "description",
		"message": "description must start with a third-person verb (e.g. 'Adds', 'Runs', 'Applies')",
		"nature":  "amplifier",
	}
}

# body must stay within 500 lines: a longer skill degrades context-window
# efficiency and hides complexity that belongs in a tool.
violation contains v if {
	count(split(input.body, "\n")) > 500
	v := {
		"field":   "body",
		"message": "body must be at most 500 lines; split the workflow or move the logic into a tool",
		"nature":  "amplifier",
	}
}

# no hardcoded secrets in the body. Pattern-based scan, deliberately naive:
# AWS access keys, PEM private keys, and inline credential assignments.
secret_patterns := {
	"AKIA[0-9A-Z]{16}",
	"-----BEGIN [A-Z ]*PRIVATE KEY",
	`(?i)(api[_-]?key|secret|password|token)\s*[:=]\s*["'][^"']+["']`,
}

violation contains v if {
	some pattern in secret_patterns
	regex.match(pattern, input.body)
	v := {
		"field":   "body",
		"message": sprintf("body contains what looks like a hardcoded secret (pattern %q); load credentials from the environment instead", [pattern]),
		"nature":  "amplifier",
	}
}
