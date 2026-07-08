# How to add a skill governance rule

> Solves one problem: adding an enterprise validation rule that every skill
> must pass at publish time (`POST /validate_skill`).

1. Pick the file: `skill-governance/structure.rego` for structural rules,
   `skill-governance/security.rego` for security rules, or a new `.rego`
   file in the same directory — every file there with
   `package ppg.skills.governance` is loaded, and violation rules union
   automatically.

2. Write a `violation contains v if {...}` rule over the skill input
   (fields: `name`, `description`, `version`, `argument_hint`, `body`,
   `rego_policy` — see the [reference](../reference/skill-governance.md)).
   Worked example — require descriptions to start with a verb-like
   capitalized word, a rule the reference article names and this PoC does
   not ship yet:

   ```rego
   package ppg.skills.governance

   import rego.v1

   violation contains v if {
       input.description
       not regex.match(`^[A-Z][a-z]+s\s`, input.description)
       v := {
           "field":   "description",
           "message": "description must start with a third-person verb (e.g. 'Applies', 'Generates')",
           "nature":  "amplifier",
       }
   }
   ```

3. Shape of the violation object: `field` (the skill field concerned),
   `message` (actionable fix), `nature` (`amplifier` or `compensatory`,
   using the 2× test; a compensatory rule should carry its sunset condition
   in a comment until the report tracks skill policies).

4. Restart the gateway (add `-skill-governance <dir>` if you use a
   non-default directory) and verify with curl:

   ```bash
   curl -s -X POST localhost:8000/validate_skill \
     -H "Content-Type: application/json" \
     -d '{"name":"demo-skill","description":"do things","version":"1.0.0","body":"Read the files."}'
   ```

   The response must list your new violation.

5. Add a test case in `internal/skill/linter_test.go`, mirroring the rule in
   `internal/skill/testdata/governance.rego` (mind the drift: the testdata
   file is a condensed copy of the production policies).
