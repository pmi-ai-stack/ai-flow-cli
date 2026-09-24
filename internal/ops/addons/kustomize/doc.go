// Package kustomize implements ops.Addon for a Kustomize overlay —
// the spec's own capability-matrix row: Detect by "kustomization.yaml";
// Render "kustomize build"; Validate "kubeconform"; Drift "kubectl
// diff -k" of the base rendering; DryRun "kubectl diff -k",
// "apply -k --dry-run=server"; Rollback and Harnesses "as kubectl" —
// read literally, meaning this package's own rollback.go/harness.go/
// kindclass.go are byte-for-byte duplicates of the sibling kubectl
// package's own (see .ai/phase-26/plan.md §0.11 for why duplication,
// not a shared package, was chosen).
//
// Render flattens a kustomize target into one plain "rendered.yaml"
// file (kustomize build's own stdout) — the same shape every other
// addon's rendering takes, so Drift/DryRun never literally shell out a
// "-k"-flagged call against the (already-flattened) rendering: there
// is no kustomization.yaml left inside it for "-k" to target. Both
// stages instead reuse kubectl's own rendering-comparison shape
// (kubectl diff's exit code as a presence-only fast path, then
// kubectl get / apply --dry-run=server for the full changeset) against
// the already-built rendering — see .ai/phase-26/plan.md §1.2.1 for
// the full reasoning.
package kustomize
