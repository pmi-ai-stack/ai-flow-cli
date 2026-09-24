package main

import rego.v1

test_deny_uppercase_local_name if {
	count(deny) > 0 with input as {"resource": {"aws_s3_bucket": {"MyBucket": [{"bucket": "x"}]}}}
}

test_deny_hyphenated_local_name if {
	count(deny) > 0 with input as {"resource": {"aws_instance": {"web-server": [{"ami": "ami-1"}]}}}
}

test_allow_snake_case_local_name if {
	# random_id: not a taggable_types member and its "random_" prefix is
	# allowed, so only this file's own rule can fire against it — a
	# clean fixture for "this rule alone allows a good name," not
	# incidentally tripping tags.rego or providers.rego too (both
	# packages combine into the same data.main.deny set conftest
	# verify/test evaluates as one).
	count(deny) == 0 with input as {"resource": {"random_id": {"my_id": [{"byte_length": 4}]}}}
}
