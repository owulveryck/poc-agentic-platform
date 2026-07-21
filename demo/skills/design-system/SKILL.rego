package ppg.skills.design_system

import rego.v1

# Companion of ADR-090. Since v1.0.0 the plan linter unions skill-companion
# policies (Gate 3) and — as of the artifact/changeset extension — the guards
# and apply-time backstop union them at the content altitudes too. Rules use
# input.view to discriminate, mirroring ADR-090.rego. See
# docs/reference/policy-views.md for the three views.
#
# Plan altitude: any plan produced under the design-system workflow that
# touches UI files must include a step reading design/tokens.css.
#
# Artifact altitude: every UI file the agent writes MUST reach visual values
# through the design tokens. Raw hex colors are refused at the moment of the
# edit, so this rule fires even when ADR-090 is not loaded in the ADR corpus.

violation contains v if {
	input.view == "plan"
	some step in input.steps
	target_is_ui_step(step)
	not plan_reads_design_tokens
	v := {
		"policy_id": "design_tokens_referenced",
		"message":   "Design-system skill: this plan touches a UI file but no step reads design/tokens.css. Add a step whose targets include \"design/tokens.css\" so the model plans against the canonical palette.",
		"nature":    "amplifier",
	}
}

target_is_ui_step(step) if endswith(step.targets[_], ".html")

target_is_ui_step(step) if endswith(step.targets[_], ".css")

target_is_ui_step(step) if endswith(step.targets[_], ".tsx")

target_is_ui_step(step) if endswith(step.targets[_], ".jsx")

target_is_ui_step(step) if endswith(step.targets[_], ".svelte")

target_is_ui_step(step) if endswith(step.targets[_], ".vue")

plan_reads_design_tokens if {
	some step in input.steps
	step.targets[_] == "design/tokens.css"
}

# Skill-scoped closure of the tokens-file bypass: a plan produced under this
# skill MUST NOT overwrite design/tokens.css from within the agent loop —
# that file is the canonical palette and belongs to a human commit outside
# any session. This mirrors ADR-120 but is scoped to the skill so the
# defence travels with the skill even when ADR-120 is not loaded in the
# corpus.
violation contains v if {
	input.view == "plan"
	some step in input.steps
	is_write_class(step.tool)
	step.targets[_] == "design/tokens.css"
	v := {
		"policy_id": "design_tokens_immutable",
		"message":   sprintf("Design-system skill: step %q would write design/tokens.css, but the palette is materialized by the skill and read by its enforcement — modifying it from within an agent session defeats the invariant. Extend it through a human git commit outside any session.", [step.id]),
		"nature":    "amplifier",
	}
}

is_write_class(t) if t == "Write"

is_write_class(t) if t == "Edit"

is_write_class(t) if t == "MultiEdit"

is_write_class(t) if t == "NotebookEdit"

is_write_class(t) if t == "Update"

is_write_class(t) if t == "create_file"

is_write_class(t) if t == "edit_file"

is_write_class(t) if t == "editFiles"

is_write_class(t) if t == "str_replace_editor"

is_write_class(t) if t == "apply_patch"

is_write_class(t) if t == "patch_code"

is_write_class(t) if contains(t, "Write")

is_write_class(t) if contains(t, "Edit")

# ---------------------------------------------------------------------------
# Content views: enforce the raw-color ban against the ACTUAL edited content,
# not just the plan. governed_files unifies the artifact view (one edit) and
# the changeset view (a whole diff) so one rule set covers both altitudes.
# design/tokens.css is exempt — it is where raw values legitimately live, and
# ADR-120 keeps agents from writing to it directly.
# ---------------------------------------------------------------------------

governed_files contains f if {
	input.view == "artifact"
	f := input.artifact
}

governed_files contains f if {
	input.view == "changeset"
	some file in input.changeset.files
	f := file
}

governed_ui_file(f) if {
	is_ui_path(f.path)
	f.path != "design/tokens.css"
}

is_ui_path(p) if endswith(p, ".html")

is_ui_path(p) if endswith(p, ".css")

is_ui_path(p) if endswith(p, ".tsx")

is_ui_path(p) if endswith(p, ".jsx")

is_ui_path(p) if endswith(p, ".svelte")

is_ui_path(p) if endswith(p, ".vue")

# hex_color_pattern matches #rgb, #rrggbb, and #rrggbbaa forms.
hex_color_pattern := `#[0-9a-fA-F]{3,8}\b`

violation contains v if {
	some f in governed_files
	governed_ui_file(f)
	regex.match(hex_color_pattern, f.content)
	v := {
		"policy_id": "design_tokens_referenced",
		"message":   sprintf("Design-system skill: %s uses a raw hex color. Route the value through a design token (var(--color-*)) declared in design/tokens.css.", [f.path]),
		"nature":    "amplifier",
	}
}

# Content-altitude closure of the tokens-file bypass. Since the union
# semantics extension, every registered skill's content rules apply to every
# edit — whether or not the plan declared this skill — so this rule protects
# the palette even for a plan that omitted skill_id (where the plan-view
# design_tokens_immutable rule above never fires). The skill's own bootstrap
# copies the file via Bash (`cp`), outside the guard's write-tool events, so
# legitimate materialization is unaffected; in-session Write/Edit of the
# palette is refused here at the moment of the edit.
violation contains v if {
	some f in governed_files
	f.path == "design/tokens.css"
	v := {
		"policy_id": "design_tokens_immutable",
		"message":   "Design-system skill: design/tokens.css is materialized by the skill and read by its enforcement — modifying it from within an agent session defeats the invariant. Extend it through a human git commit outside any session.",
		"nature":    "amplifier",
	}
}
