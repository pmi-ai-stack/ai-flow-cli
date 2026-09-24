// Package githubactions implements ops.Addon for GitHub Actions
// workflow files — the spec's own capability-matrix row: Detect
// ".github/workflows/*.yml"; Render copy; Validate "actionlint";
// Drift "not_applicable" (Capabilities().Drift is false — see addon.go);
// DryRun "act -n" job graph + a per-job structural diff between the
// base and branch renderings; Rollback a text-only "previous file,
// revert commit" instruction; Harnesses "act -j <job> -n" (unit) /
// "act -j <job>" (e2e, opt-in) detection — running them is deferred,
// see .ai/phase-27/plan.md §1.2.1.
//
// See .ai/phase-27/plan.md for the design decisions behind every
// method, in particular why DryRun never emits a "noop" action (design
// decision 5) and why Capabilities().Drift is false rather than a
// method that always returns the same constant (§1.1.1).
package githubactions
