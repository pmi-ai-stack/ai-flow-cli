# Verified with a real conftest binary — see naming.rego's own header
# comment.
package main

import rego.v1

# taggable_types is deliberately a short, named allowlist rather than
# "every aws_* type" — several common types (aws_iam_policy_attachment,
# aws_route_table_association) accept no tags argument at all, so a
# blanket rule would misfire on every one of them. Extend this set in a
# team or repo layer's own override as more taggable types matter.
taggable_types := {"aws_instance", "aws_s3_bucket", "aws_db_instance", "aws_vpc"}

# deny_missing_tags: every resource of a taggable type must set a
# non-empty tags map — infra/terraform/guidance/baseline.md rule 2.
deny contains msg if {
	some rtype, rname
	rtype in taggable_types
	some cfg in input.resource[rtype][rname]
	not has_tags(cfg)
	msg := sprintf("terraform resource %s.%s: must set a non-empty tags map", [rtype, rname])
}

has_tags(cfg) if {
	count(object.get(cfg, "tags", {})) > 0
}
