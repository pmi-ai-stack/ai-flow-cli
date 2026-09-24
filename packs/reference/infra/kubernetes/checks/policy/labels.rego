# Verified with a real conftest binary — see naming.rego's own header
# comment.
package main

import rego.v1

workload_kinds := {"Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "Pod"}

# deny_missing_name_label: every workload must carry
# app.kubernetes.io/name — infra/kubernetes/guidance/baseline.md rule 2.
deny contains msg if {
	input.kind in workload_kinds
	not input.metadata.labels["app.kubernetes.io/name"]
	msg := sprintf("%s/%s: missing required label \"app.kubernetes.io/name\"", [input.kind, input.metadata.name])
}
