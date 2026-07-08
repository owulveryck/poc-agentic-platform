package ppg.skills.governance

import rego.v1

violation contains v if {
	not input.name
	v := {"field": "name", "message": "name is required", "nature": "amplifier"}
}

violation contains v if {
	input.name
	not regex.match(`^[a-z][a-z0-9-]{0,31}$`, input.name)
	v := {"field": "name", "message": "name must be lowercase-kebab-case and at most 32 characters", "nature": "amplifier"}
}

violation contains v if {
	not input.description
	v := {"field": "description", "message": "description is required", "nature": "amplifier"}
}

violation contains v if {
	input.description
	count(input.description) < 50
	v := {"field": "description", "message": "description must be at least 50 characters to be discoverable", "nature": "amplifier"}
}

violation contains v if {
	not input.version
	v := {"field": "version", "message": "version is required for registry publication", "nature": "amplifier"}
}

violation contains v if {
	contains(input.body, "$ARGUMENTS")
	not input.argument_hint
	v := {"field": "argument_hint", "message": "argument-hint is required when the skill body uses $ARGUMENTS", "nature": "amplifier"}
}

modifies_files if { contains(input.body, "Edit") }
modifies_files if { contains(input.body, "Write") }

violation contains v if {
	modifies_files
	not input.rego_policy
	v := {"field": "rego_policy", "message": "Skills that instruct file modifications must include a companion SKILL.rego.", "nature": "amplifier"}
}
