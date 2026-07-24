package ppg.linter

import rego.v1

# ADR-211: a BMAD dev plan is planned against its story. Plan altitude only: a
# plan that writes implementation code must also read the story it implements.
# Mirrors ADR-203.rego (API changes are contract-first).

violation contains v if {
	input.view == "plan"
	some step in input.steps
	adr211_targets_impl(step)
	not adr211_plan_reads_story
	v := {
		"policy_id": "bmad_plan_references_story",
		"message": "BMAD invariant: this plan writes implementation code (a step targets src/) but no step reads the story it implements. Add a step whose targets include the story file (e.g. _bmad-output/implementation-artifacts/<story>.md, Read is enough) — the story is the Dev agent's contract.",
		"nature": "amplifier",
	}
}

adr211_targets_impl(step) if {
	some t in step.targets
	contains(lower(t), "src/")
}

adr211_plan_reads_story if {
	some step in input.steps
	some t in step.targets
	adr211_is_story(lower(t))
}

adr211_is_story(t) if contains(t, "implementation-artifacts/")

adr211_is_story(t) if contains(t, "/stories/")

adr211_is_story(t) if endswith(t, ".story.md")
