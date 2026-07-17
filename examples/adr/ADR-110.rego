package ppg.linter

import rego.v1

# ADR-110: integrate shared capabilities through the platform-cataloged service.
#
# The deterministic guarantee is at the content altitudes (artifact + changeset):
# written code containing a forbidden/deprecated provider marker is denied, with
# a message naming the sanctioned service to use instead. A narrow plan-view rule
# also catches a plan that names a forbidden provider outright.
#
# forbidden_markers is kept in sync BY HAND with the catalog records under
# services/ (status: forbidden|deprecated). See
# docs/how-to/add-a-service-to-the-catalog.md.

# governed_files unifies the artifact view (one edit) and the changeset view
# (a whole diff). Redeclared here (identically to ADR-090) so this policy is
# self-contained; partial set rules union across files in the same package.
governed_files contains f if {
	input.view == "artifact"
	f := input.artifact
}

governed_files contains f if {
	input.view == "changeset"
	some file in input.changeset.files
	f := file
}

forbidden_markers := {
	"github.com/stripe/stripe-go": "the Payments Gateway (payments-gateway, http://localhost:9120) — route payments through the gateway per ADR-042, not the Stripe SDK.",
	"api.stripe.com": "the Payments Gateway (payments-gateway, http://localhost:9120) — route payments through the gateway per ADR-042, not api.stripe.com directly.",
	"legacy-mailer.internal": "the Notification Service (notify-svc, http://localhost:9110) — legacy-mailer is deprecated.",
}

violation contains v if {
	some f in governed_files
	some marker, guidance in forbidden_markers
	contains(f.content, marker)
	v := {
		"policy_id": "use_cataloged_services",
		"message": sprintf("Forbidden provider in %s: %q is not permitted. Use %s Call find_platform_service to discover the sanctioned service.", [f.path, marker, guidance]),
		"nature": "amplifier",
	}
}

violation contains v if {
	input.view == "plan"
	some step in input.steps
	contains(lower(step.action), "stripe")
	v := {
		"policy_id": "use_cataloged_services",
		"message": "This plan names Stripe directly: integrate the platform Payments Gateway (payments-gateway, ADR-042) instead of a provider SDK. Call find_platform_service to get its endpoint and API usage.",
		"nature": "amplifier",
	}
}
