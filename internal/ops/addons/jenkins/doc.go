// Package jenkins implements ops.Addon for Jenkins pipeline
// definitions — the spec's own capability-matrix row: Detect
// "Jenkinsfile, job DSL, jobs/*.xml"; Render copy; Validate the
// declarative linter (Jenkinsfile targets only — see validate.go);
// Drift "get-job config against the repository" (base rendering,
// compared against a real, live jenkins-cli call); DryRun a
// structural diff (the identical live jenkins-cli call, compared
// against the *branch* rendering instead — see dryrun.go's own doc
// comment for why this, not a base-vs-branch compare, is the
// architecturally consistent reading); Rollback a text-only "previous
// job config, revert commit" instruction; Harnesses JenkinsPipelineUnit
// (unit) / jenkinsfile-runner (e2e, opt-in) detection.
//
// "Jenkins CLI reachability" is this phase's own named open question
// (spec-v7/implementation-plan.md's Phase 27 "Decides" line) — no real
// Jenkins server or jenkins-cli binary was reachable to verify any
// invocation shape against (see .ai/phase-27/plan.md §0.1); every
// jenkins-cli call in this package is flagged provisional in its own
// doc comment, the same disclosed posture phase 22's kubeconform and
// phase 26's helm-diff/ct/kuttl/chainsaw already carry.
package jenkins
