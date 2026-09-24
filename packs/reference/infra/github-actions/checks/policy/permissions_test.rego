package main

import rego.v1

test_deny_job_with_no_permissions_and_no_workflow_default if {
	count(deny) == 1 with input as {"jobs": {"build": {"runs-on": "ubuntu-latest"}}}
}

test_allow_job_with_its_own_permissions if {
	count(deny) == 0 with input as {"jobs": {"build": {"runs-on": "ubuntu-latest", "permissions": {"contents": "read"}}}}
}

test_allow_workflow_level_default_covers_every_job if {
	count(deny) == 0 with input as {
		"permissions": {"contents": "read"},
		"jobs": {"build": {"runs-on": "ubuntu-latest"}, "test": {"runs-on": "ubuntu-latest"}},
	}
}

test_deny_one_of_two_jobs_missing_permissions if {
	count(deny) == 1 with input as {"jobs": {
		"build": {"runs-on": "ubuntu-latest", "permissions": {"contents": "read"}},
		"test": {"runs-on": "ubuntu-latest"},
	}}
}
