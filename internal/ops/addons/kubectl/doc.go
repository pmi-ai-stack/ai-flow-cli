// Package kubectl implements ops.Addon for a directory of raw
// Kubernetes manifests — the spec's own capability-matrix row: Detect
// by apiVersion+kind outside charts/overlays; Render a plain copy;
// Validate "kubeconform"; Drift "kubectl diff" of the base rendering;
// DryRun "kubectl diff --server-side" (a presence-only fast path)
// followed by "kubectl get"/"apply --dry-run=server" (both -o json)
// when it reports a difference, normalized into a Changeset; Rollback
// "kubectl rollout history" (a real, read-only call — unlike
// terraform's own zero-subprocess Rollback, see .ai/phase-22/plan.md's
// design decision 4) followed by a text-only, never-executed "kubectl
// rollout undo" instruction; Harnesses detects "kuttl-test.yaml" and
// "chainsaw-test.yaml" files (running them is phase 24's
// internal/ops/harness, not this package — Capabilities().UnitTest is
// false, the capability matrix's own Unit-harness cell is "—").
//
// See .ai/phase-22/plan.md for the design decisions behind every
// method — in particular why Detect cannot reuse filepath.WalkDir's
// own SkipDir mechanism (design decision 1), why an unrecognized kind
// defaults to KindOther rather than terraform's own KindWorkload
// (design decision 3), and why Action.Sensitive is always false from
// this addon (design decision 5).
package kubectl
