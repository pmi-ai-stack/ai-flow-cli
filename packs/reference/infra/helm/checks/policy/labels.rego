# Verified with a real conftest binary (`conftest verify` and
# `conftest test --output json` against a real "helm template"
# rendering of internal/ops/addons/helm/testdata/chart — see
# .ai/phase-26/implementation-log.md), against conftest's own
# documented YAML input shape: one manifest document per evaluation,
# the document itself as `input`. infra.helm.policy runs with
# input: "render" (never "source" — a Helm template file is not valid
# YAML before rendering, confirmed by a real conftest run aborting
# outright on one; see .ai/phase-26/plan.md §0.4), so `input` here is
# always an already-rendered, already-substituted manifest document —
# the same input shape infra/kubernetes/checks/policy's own rego
# files already assume.
package main

import rego.v1

# deny_missing_helm_labels: every rendered resource must carry both of
# Helm's own documented recommended labels
# (https://helm.sh/docs/chart_best_practices/labels/) —
# "helm.sh/chart" and "app.kubernetes.io/managed-by" —
# infra/helm/guidance/baseline.md rule 5. Genuinely Helm-specific, not
# already covered by infra/kubernetes/checks/policy/labels.rego's own
# "app.kubernetes.io/name" rule, which applies to every workload
# regardless of which addon rendered it.
required_helm_labels := {"helm.sh/chart", "app.kubernetes.io/managed-by"}

deny contains msg if {
	some label in required_helm_labels
	not input.metadata.labels[label]
	msg := sprintf("%s/%s: missing required label %q (Helm's own recommended-labels convention)", [input.kind, input.metadata.name, label])
}
