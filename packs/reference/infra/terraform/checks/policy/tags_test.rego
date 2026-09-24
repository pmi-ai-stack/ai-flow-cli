package main

import rego.v1

test_deny_missing_tags if {
	count(deny) > 0 with input as {"resource": {"aws_instance": {"web": [{"ami": "ami-1"}]}}}
}

test_deny_empty_tags if {
	count(deny) > 0 with input as {"resource": {"aws_instance": {"web": [{"ami": "ami-1", "tags": {}}]}}}
}

test_allow_with_tags if {
	count(deny) == 0 with input as {"resource": {"aws_instance": {"web": [{"ami": "ami-1", "tags": {"Name": "web"}}]}}}
}

test_allow_non_taggable_type if {
	count(deny) == 0 with input as {"resource": {"random_id": {"suffix": [{"byte_length": 4}]}}}
}
