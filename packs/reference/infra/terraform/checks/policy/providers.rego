# Verified with a real conftest binary — see naming.rego's own header
# comment.
package main

import rego.v1

# allowed_prefixes: "allowed providers and registries" —
# infra/terraform/guidance/baseline.md rule 3. A team that genuinely
# needs another provider declares it in a team or repo override
# (standards.json's own overrides[]); the org layer does not loosen
# this in place.
allowed_prefixes := {"aws_", "random_", "null_", "local_"}

deny contains msg if {
	some rtype
	input.resource[rtype]
	not allowed_prefix(rtype)
	msg := sprintf("terraform resource type %q is not from an approved provider (allowed prefixes: aws_, random_, null_, local_)", [rtype])
}

allowed_prefix(rtype) if {
	some prefix in allowed_prefixes
	startswith(rtype, prefix)
}
