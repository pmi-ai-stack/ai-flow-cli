# Verified with a real conftest binary — see naming.rego's own header
# comment.
package main

import rego.v1

# pod_spec: the pod template's own spec, located differently per kind —
# a Pod's spec is at the top level, a CronJob's is nested two levels
# deeper than every other workload kind's. Multiple definitions, first
# matching kind wins; a kind none of these three match (ConfigMap,
# Service, ...) leaves pod_spec undefined, which the deny rule below
# fails on cleanly rather than erroring.
pod_spec(kind) := spec if {
	kind in {"Deployment", "StatefulSet", "DaemonSet", "Job"}
	spec := input.spec.template.spec
}

pod_spec(kind) := spec if {
	kind == "CronJob"
	spec := input.spec.jobTemplate.spec.template.spec
}

pod_spec(kind) := spec if {
	kind == "Pod"
	spec := input.spec
}

# deny_unpinned_image: no container or init container may use an
# untagged or `:latest` image — infra/kubernetes/guidance/baseline.md
# rule 3.
deny contains msg if {
	spec := pod_spec(input.kind)
	some container in array.concat(object.get(spec, "containers", []), object.get(spec, "initContainers", []))
	bad_image(container.image)
	msg := sprintf("%s/%s: container %q uses image %q without a pinned tag", [input.kind, input.metadata.name, container.name, container.image])
}

bad_image(image) if {
	endswith(image, ":latest")
}

bad_image(image) if {
	not contains(image, ":")
}
