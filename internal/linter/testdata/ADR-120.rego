package ppg.linter

import rego.v1

# ADR-120: governance artifacts (skill bodies, canonical design-system files)
# may not appear as write/edit targets in a locked plan. Reads are allowed.
# Plan altitude only — no artifact/changeset view. Plan rejection means the
# ticket is never minted, and the empty-ticket path in ppg-guard blocks any
# subsequent Write on the same session.

governance_paths_exact := {"design/tokens.css"}

governance_paths_prefix := {".claude/skills/", ".agents/skills/"}

# Mirrors isWriteTool in adapters/claudecode/guard/main.go. Keep in sync when
# that list changes.
write_tools := {
	"Write", "Edit", "MultiEdit", "NotebookEdit", "Update",
	"create_file", "edit_file", "editFiles", "str_replace_editor", "apply_patch",
	"patch_code",
}

is_write_tool(t) if write_tools[t]

is_write_tool(t) if contains(t, "Write")

is_write_tool(t) if contains(t, "Edit")

is_governance_path(p) if governance_paths_exact[p]

is_governance_path(p) if {
	some prefix in governance_paths_prefix
	startswith(p, prefix)
}

violation contains v if {
	input.view == "plan"
	some step in input.steps
	is_write_tool(step.tool)
	some target in step.targets
	is_governance_path(target)
	v := {
		"policy_id": "governance_artifacts_immutable",
		"message": sprintf(
			"Governance-artifact invariant: step %q would %s %q, a platform-canonical file. Skill definitions (.claude/skills/, .agents/skills/) and the design tokens file (design/tokens.css) are materialized by the platform and read by ADR enforcement — modifying them from within an agent session defeats the invariants they carry. Extend them through a human git commit outside an agent session.",
			[step.id, step.tool, target],
		),
		"nature": "amplifier",
	}
}
