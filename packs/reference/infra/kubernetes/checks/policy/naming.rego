# Verified with a real conftest binary (`conftest verify` and `conftest
# test --output json` against a throwaway .yaml fixture — see
# .ai/phase-25/implementation-log.md), against conftest's own
# documented YAML input shape: one manifest document per evaluation,
# the document itself as `input`.
package main

import rego.v1

max_name_length := 63

# deny_name_too_long / deny_name_invalid_shape:
# infra/kubernetes/guidance/baseline.md rule 1.
deny contains msg if {
	input.metadata.name
	count(input.metadata.name) > max_name_length
	msg := sprintf("%s/%s: metadata.name exceeds %d characters (RFC 1123 label limit)", [input.kind, input.metadata.name, max_name_length])
}

deny contains msg if {
	input.metadata.name
	not regex.match(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, input.metadata.name)
	msg := sprintf("%s/%s: metadata.name must be a lowercase RFC 1123 label (letters, digits, hyphens)", [input.kind, input.metadata.name])
}
