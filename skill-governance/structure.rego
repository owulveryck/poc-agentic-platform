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
