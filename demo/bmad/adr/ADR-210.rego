package ppg.linter

import rego.v1

# ADR-210: a BMAD story file carries its mandatory sections (Acceptance Criteria
# + Tasks). Content altitudes (artifact + changeset). Compensatory tripwire
# against a story that reaches the Dev agent silently truncated.

# Unify the two content views so one rule set covers both, exactly like
# ADR-205.rego / examples/adr/ADR-090.rego.
governed_files contains f if {
	input.view == "artifact"
	f := input.artifact
}

governed_files contains f if {
	input.view == "changeset"
	some file in input.changeset.files
	f := file
}

# A file is a BMAD story when it lives where BMAD writes stories, or is named
# *.story.md. (BMAD default_output_file: {implementation_artifacts}/{story_key}.md.)
adr210_is_story(p) if contains(lower(p), "implementation-artifacts/")

adr210_is_story(p) if contains(lower(p), "/stories/")

adr210_is_story(p) if endswith(lower(p), ".story.md")

# (A) The story must declare its Acceptance Criteria section.
violation contains v if {
	some f in governed_files
	adr210_is_story(f.path)
	not regex.match(`(?im)^##\s+Acceptance Criteria\b`, f.content)
	v := {
		"policy_id": "bmad_story_schema_complete",
		"message": sprintf("BMAD story invariant (%s): missing the '## Acceptance Criteria' section. A story handed to the Dev agent without acceptance criteria is not a BMAD story — the create-story template requires it.", [f.path]),
		"nature": "compensatory",
	}
}

# (B) The story must declare its Tasks / Subtasks section.
violation contains v if {
	some f in governed_files
	adr210_is_story(f.path)
	not regex.match(`(?im)^##\s+Tasks\b`, f.content)
	v := {
		"policy_id": "bmad_story_schema_complete",
		"message": sprintf("BMAD story invariant (%s): missing the '## Tasks / Subtasks' section. The Dev agent works the task list; a story without it has no executable plan.", [f.path]),
		"nature": "compensatory",
	}
}
