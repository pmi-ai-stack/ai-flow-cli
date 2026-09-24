package main

import rego.v1

test_deny_missing_label if {
	count(deny) > 0 with input as {"kind": "Deployment", "metadata": {"name": "web"}}
}

test_allow_with_label if {
	count(deny) == 0 with input as {"kind": "Deployment", "metadata": {"name": "web", "labels": {"app.kubernetes.io/name": "web"}}}
}

test_allow_non_workload_kind if {
	count(deny) == 0 with input as {"kind": "ConfigMap", "metadata": {"name": "cfg"}}
}
