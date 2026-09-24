package main

import rego.v1

test_deny_latest_tag if {
	count(deny) > 0 with input as {
		"kind": "Deployment", "metadata": {"name": "web"},
		"spec": {"template": {"spec": {"containers": [{"name": "app", "image": "nginx:latest"}]}}},
	}
}

test_deny_untagged_image if {
	count(deny) > 0 with input as {
		"kind": "Deployment", "metadata": {"name": "web"},
		"spec": {"template": {"spec": {"containers": [{"name": "app", "image": "nginx"}]}}},
	}
}

test_allow_pinned_tag if {
	# A Deployment also needs app.kubernetes.io/name (labels.rego) —
	# both files' rules combine into one data.main.deny set, so a
	# fixture proving "a pinned tag draws no images.rego denial" must
	# still satisfy every other rule that applies to a Deployment.
	count(deny) == 0 with input as {
		"kind": "Deployment", "metadata": {"name": "web", "labels": {"app.kubernetes.io/name": "web"}},
		"spec": {"template": {"spec": {"containers": [{"name": "app", "image": "nginx:1.27.0"}]}}},
	}
}

test_allow_non_workload_kind if {
	count(deny) == 0 with input as {"kind": "ConfigMap", "metadata": {"name": "cfg"}}
}

test_deny_pod_kind_untagged if {
	count(deny) > 0 with input as {
		"kind": "Pod", "metadata": {"name": "debug"},
		"spec": {"containers": [{"name": "shell", "image": "busybox"}]},
	}
}
