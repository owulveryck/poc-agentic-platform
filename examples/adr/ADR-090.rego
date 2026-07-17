package ppg.linter

import rego.v1

# ADR-090: any plan that touches UI files must include a step that reads
# the design tokens — the model must acknowledge the tokens exist before
# planning visual changes.

violation contains v if {
	input.view == "plan"
	some step in input.steps
	target_is_ui(step)
	not plan_reads_design_tokens
	v := {
		"policy_id": "design_tokens_referenced",
		"message":   "Design-system invariant: this plan touches a UI file but no step reads design/tokens.css. Add a step whose targets include \"design/tokens.css\" (Read is enough) so the model plans against the canonical palette.",
		"nature":    "amplifier",
	}
}

target_is_ui(step) if {
	endswith(step.targets[_], ".html")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".css")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".tsx")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".jsx")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".svelte")
}

target_is_ui(step) if {
	endswith(step.targets[_], ".vue")
}

plan_reads_design_tokens if {
	some step in input.steps
	step.targets[_] == "design/tokens.css"
}

# ---------------------------------------------------------------------------
# Content views: enforce the token invariant against the ACTUAL edited content,
# not just the plan. governed_files unifies the artifact view (one edit) and the
# changeset view (a whole diff), so a single content-rule set covers both
# altitudes. This subsumes the former design-guard.sh hook and closes its
# audited bypasses: var() fallbacks with a raw color, named colors beyond a
# short list, and button rules hidden behind pseudo-classes/combinators. The
# tokens file itself is exempt — it is where raw values legitimately live.
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

violation contains v if {
	some f in governed_files
	governed_ui_file(f)
	raw_color_present(lower(f.content))
	v := {
		"policy_id": "design_tokens_referenced",
		"message":   sprintf("Design-system invariant (%s): raw color value found. Reach colors through design tokens (var(--color-*)) or a CSS keyword (transparent, inherit, currentColor, unset, initial); raw hex, rgb()/hsl(), and named colors are forbidden outside design/tokens.css.", [f.path]),
		"nature":    "amplifier",
	}
}

# raw_color_present intentionally does NOT strip var() first, so a raw color in
# a var() fallback (var(--x, #F0F)) is still caught.
raw_color_present(c) if regex.match(`#[0-9a-f]{3,8}\b`, c)

raw_color_present(c) if regex.match(`(rgb|hsl|hwb|lab|lch)a?\(`, c)

raw_color_present(c) if {
	regex.match(`:\s*(aqua|beige|black|blue|brown|chocolate|coral|crimson|cyan|fuchsia|gold|goldenrod|gray|grey|green|hotpink|indigo|ivory|khaki|lavender|lime|magenta|maroon|navy|olive|orange|orchid|pink|plum|purple|red|salmon|silver|snow|tan|teal|tomato|turquoise|violet|wheat|white|yellow)\b`, c)
}

violation contains v if {
	some f in governed_files
	governed_ui_file(f)
	button_rule_present(f.content)
	v := {
		"policy_id": "design_tokens_referenced",
		"message":   sprintf("Design-system invariant (%s): button styling belongs in design/tokens.css only. Use <button> or class=\"btn\" in markup; do not re-declare button geometry here. Extend design/tokens.css if a new variant is genuinely needed.", [f.path]),
		"nature":    "amplifier",
	}
}

# A button style rule: the selector token 'button', '.btn', '.button', or
# [role="button"] followed (allowing pseudo-classes/combinators/attributes) by
# an opening brace. Closes the button:hover / button > span bypasses.
button_rule_present(c) if regex.match(`(^|[^-_a-z0-9])button\b[^{};]*\{`, lower(c))

button_rule_present(c) if regex.match(`\.btn\b[^{};]*\{`, lower(c))

button_rule_present(c) if regex.match(`\.button\b[^{};]*\{`, lower(c))

button_rule_present(c) if regex.match(`\[role=.?button.?\][^{};]*\{`, lower(c))
