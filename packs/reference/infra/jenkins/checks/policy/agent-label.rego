# Verified with a real conftest binary against a real job-config XML
# fixture and a real Jenkinsfile (Groovy) — see
# .ai/phase-27/implementation-log.md. infra.jenkins.policy runs with
# input: "render", a byte-identical copy of the target's own source
# (Render never transforms any of this addon's three shapes). conftest
# genuinely parses XML into navigable structure (confirmed for real);
# a Jenkinsfile's own Groovy content parses too, but never produces an
# "input.project" key — the guard below (`input.project` first) is
# load-bearing, not defensive style: without it this rule would fire
# against every Jenkinsfile in existence, confirmed for real by
# removing the guard and re-running against a real Jenkinsfile fixture
# (see infra/jenkins/guidance/baseline.md's own closing line).
package main

import rego.v1

# deny_missing_agent_label: a job-config XML's own <assignedNode> must
# be set — an unset agent label lets Jenkins schedule the job on any
# connected agent, including one outside the team's own trust boundary
# on a shared/multi-tenant controller (infra/jenkins/guidance/baseline.md
# rule 1). Only ever fires against a real job-config XML target
# (jobs/*.xml) — never against a Jenkinsfile or a Job DSL script,
# neither of which this rego can honestly evaluate (see the guard
# above).
deny contains msg if {
	input.project
	not input.project.assignedNode
	msg := "job config has no <assignedNode> — an explicit agent label is required on a shared Jenkins controller"
}
