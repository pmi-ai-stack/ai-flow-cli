package main

import rego.v1

test_deny_disallowed_provider if {
	count(deny) > 0 with input as {"resource": {"google_compute_instance": {"vm": [{}]}}}
}

test_allow_aws_provider if {
	# aws_instance is a taggable_types member, so this fixture also
	# carries tags — otherwise tags.rego's own deny would fire too
	# (both files' rules combine into one data.main.deny set).
	count(deny) == 0 with input as {"resource": {"aws_instance": {"web": [{"tags": {"Name": "web"}}]}}}
}

test_allow_random_provider if {
	count(deny) == 0 with input as {"resource": {"random_id": {"suffix": [{}]}}}
}
