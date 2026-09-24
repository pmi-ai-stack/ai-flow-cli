# Verified with a real conftest binary (`conftest verify` and
# `conftest test --output json` against a real workflow YAML fixture —
# see .ai/phase-27/implementation-log.md). infra.github-actions.policy
# runs with input: "render", which for this addon is a byte-identical
# copy of the workflow's own source (Render never templatizes GitHub
# Actions YAML the way Helm's own Render does) — the same input shape
# infra/kubernetes/checks/policy's own rego files already assume: the
# workflow document itself as `input`.
package main

import rego.v1

# deny_missing_permissions: every job must carry its own
# "permissions:" block, or the workflow itself must declare one at the
# top level covering every job — GitHub's own default token, when
# neither is set, is read-write on every scope it can reach
# (infra/github-actions/guidance/baseline.md rule 1). This must-severity
# structural gate and infra.github-actions.scan's own zizmor
# "excessive-permissions" finding (should-severity) name the identical
# underlying risk through two different tools, the same relationship
# infra.kubernetes.policy/infra.kubernetes.scan already have.
deny contains msg if {
	not input.permissions
	some job_id, job in input.jobs
	not job.permissions
	msg := sprintf("job %q has no \"permissions:\" block, and the workflow declares no top-level default", [job_id])
}
