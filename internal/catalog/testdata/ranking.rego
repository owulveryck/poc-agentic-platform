package ppg.catalog

# Ranking policy for the platform Service Catalog (policy-as-code).
#
# Input:  {capability, repository_context, candidates: [<service>, ...]}
# Output: verdict[] — one decision per candidate:
#           {service_id, allow, score, reason}
#
# The gateway (internal/catalog.Ranker) sorts allowed candidates by score
# (then tier) to pick the recommended service, and surfaces denied ones as
# alternatives with their reason. A candidate the policy returns NO verdict for
# is treated as denied by the caller (fail-closed), so an unknown status never
# silently becomes recommendable.
#
# This is where an organization encodes "which is the best one": tighten it
# with region/compliance/cost rules over svc.policy_tags and
# input.repository_context without touching Go.

import rego.v1

verdict contains v if {
	some svc in input.candidates
	v := decide(svc)
}

# Forbidden: never allowed, whatever else is true.
decide(svc) := v if {
	svc.status == "forbidden"
	v := {
		"service_id": svc.service_id,
		"allow": false,
		"score": 0,
		"reason": sprintf("%s is forbidden by platform policy for %q; use the recommended service instead.", [svc.service_id, svc.capability]),
	}
}

# Deprecated: excluded from recommendation, surfaced as a superseded alternative.
decide(svc) := v if {
	svc.status == "deprecated"
	v := {
		"service_id": svc.service_id,
		"allow": false,
		"score": 0,
		"reason": superseded_reason(svc),
	}
}

# Recommended: the platform-blessed default.
decide(svc) := v if {
	svc.status == "recommended"
	v := {
		"service_id": svc.service_id,
		"allow": true,
		"score": score_for(svc, 100),
		"reason": "recommended platform service.",
	}
}

# Allowed: sanctioned but not the default.
decide(svc) := v if {
	svc.status == "allowed"
	v := {
		"service_id": svc.service_id,
		"allow": true,
		"score": score_for(svc, 50),
		"reason": "allowed platform service.",
	}
}

# Sandbox: usable for experiments, not production-blessed.
decide(svc) := v if {
	svc.status == "sandbox"
	v := {
		"service_id": svc.service_id,
		"allow": true,
		"score": score_for(svc, 10),
		"reason": "sandbox service: fine for experiments, not production-blessed.",
	}
}

# Tie-break: a lower tier number means higher priority, so subtract it.
score_for(svc, base) := base - svc.tier if is_number(svc.tier)

score_for(svc, base) := base if not is_number(svc.tier)

superseded_reason(svc) := sprintf("%s is deprecated; superseded by %s.", [svc.service_id, svc.superseded_by]) if {
	svc.superseded_by != ""
}

superseded_reason(svc) := sprintf("%s is deprecated.", [svc.service_id]) if {
	not svc.superseded_by
}
