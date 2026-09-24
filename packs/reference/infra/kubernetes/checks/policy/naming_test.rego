package main

import rego.v1

test_deny_uppercase_name if {
	count(deny) > 0 with input as {"kind": "Deployment", "metadata": {"name": "MyApp"}}
}

test_deny_underscore_name if {
	count(deny) > 0 with input as {"kind": "Deployment", "metadata": {"name": "my_app"}}
}

test_deny_too_long_name if {
	long_name := "a-very-long-resource-name-that-goes-on-and-on-and-exceeds-the-limit-easily"
	count(deny) > 0 with input as {"kind": "Deployment", "metadata": {"name": long_name}}
}

test_allow_valid_name if {
	# ConfigMap is not a workload_kinds member, so labels.rego's own
	# rule cannot fire against it — a clean fixture for "this rule
	# alone allows a good name" (both files' rules combine into one
	# data.main.deny set conftest verify/test evaluates as one).
	count(deny) == 0 with input as {"kind": "ConfigMap", "metadata": {"name": "my-app"}}
}
