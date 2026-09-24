package main

import rego.v1

test_deny_missing_both_helm_labels if {
	count(deny) == 2 with input as {"kind": "Deployment", "metadata": {"name": "web", "labels": {"app.kubernetes.io/name": "web"}}}
}

test_deny_missing_managed_by_only if {
	count(deny) == 1 with input as {"kind": "Deployment", "metadata": {"name": "web", "labels": {"helm.sh/chart": "web-1.0.0"}}}
}

test_allow_both_helm_labels_present if {
	count(deny) == 0 with input as {"kind": "Deployment", "metadata": {"name": "web", "labels": {
		"helm.sh/chart": "web-1.0.0",
		"app.kubernetes.io/managed-by": "Helm",
	}}}
}
