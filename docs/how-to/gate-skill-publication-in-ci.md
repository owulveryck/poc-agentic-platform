# How to gate skill publication in CI

> Solves one problem: making `POST /validate_skill` the publish gate (Gate 1)
> of your skills registry, so no skill reaches the organization without
> passing governance.

1. Convention: the skill repository contains `SKILL.md` (YAML front matter +
   body) and, for tier ≥ 1 skills, a companion `SKILL.rego`.

2. Build the JSON payload from the files. With `yq` and `jq`:

   ```bash
   FRONT=$(yq --front-matter=extract -o=json '.' SKILL.md)
   BODY=$(awk 'c==2 {print} /^---$/ {c++}' SKILL.md)
   REGO=$(cat SKILL.rego 2>/dev/null || true)
   jq -n --argjson fm "$FRONT" --arg body "$BODY" --arg rego "$REGO" \
     '{name: $fm.name, description: $fm.description, version: ($fm.version // empty),
       argument_hint: ($fm["argument-hint"] // empty), body: $body}
      + (if $rego == "" then {} else {rego_policy: $rego} end)' > payload.json
   ```

3. CI step (GitHub Actions): call the gateway, fail the job on rejection,
   print the violations:

   ```yaml
   - name: Validate skill against the governance gate
     run: |
       HTTP=$(curl -s -o response.json -w "%{http_code}" \
         -X POST "$PPG_URL/validate_skill" \
         -H "Content-Type: application/json" \
         --data @payload.json)
       cat response.json | jq .
       test "$HTTP" = "200"
   ```

4. Route by tier: parse `tier` from the response and require human review
   for privileged skills:

   ```yaml
   - name: Require human review for tier 2 skills
     run: |
       TIER=$(jq -r '.tier' response.json)
       if [ "$TIER" -ge 2 ]; then
         gh pr edit "$PR_NUMBER" --add-label "needs-security-review"
       fi
   ```

5. Scope note: this covers **Gate 1** (publish). Gate 2 (revalidation at
   install against possibly-tightened policies) and Gate 3 (companion Rego
   enforced at `lock_in_plan` via a plan `skill_id`) are described in the
   reference article but not implemented in this PoC — see `AUDIT.md`.
