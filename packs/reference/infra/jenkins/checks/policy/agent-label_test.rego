package main

import rego.v1

test_deny_job_config_missing_assigned_node if {
	count(deny) == 1 with input as {"project": {"description": "build job"}}
}

test_allow_job_config_with_assigned_node if {
	count(deny) == 0 with input as {"project": {"assignedNode": "linux", "description": "build job"}}
}

test_allow_non_xml_target_never_fires if {
	# A Jenkinsfile's own parsed input carries no "project" key at all —
	# the guard must make this a real no-op, not a false positive.
	count(deny) == 0 with input as {"pipeline": {"agent": "any"}}
}
