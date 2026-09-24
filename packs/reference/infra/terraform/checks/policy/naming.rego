# Verified with a real conftest binary (`conftest verify` and `conftest
# test --output json` against a throwaway .tf fixture — see
# .ai/phase-25/implementation-log.md), against conftest's own
# documented HCL2 input shape: input.resource.<type>.<name> is an array
# of one object per block instance.
package main

import rego.v1

# deny_bad_resource_name: a Terraform resource's own local name (the
# second label, "web" in `resource "aws_instance" "web" {}`) must be
# snake_case — infra/terraform/guidance/baseline.md rule 1.
deny contains msg if {
	some rtype, rname
	some _ in input.resource[rtype][rname]
	not is_snake_case(rname)
	msg := sprintf("terraform resource %s.%s: local name must be snake_case (lowercase letters, digits, underscores)", [rtype, rname])
}

is_snake_case(name) if {
	regex.match(`^[a-z][a-z0-9_]*$`, name)
}
