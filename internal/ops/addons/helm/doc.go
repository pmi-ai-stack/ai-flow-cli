// Package helm implements ops.Addon for a Helm chart — the spec's own
// capability-matrix row: Detect by "Chart.yaml" + values; Render "helm
// template"; Validate "helm lint", "kubeconform"; Drift "helm get
// manifest" against the base template; DryRun "helm diff upgrade",
// else "template + kubectl diff"; Rollback "helm history" → "helm
// rollback" command as text; Unit harness "helm unittest"/"ct lint";
// E2e harness "ct install" on "kind".
//
// Render writes two things into its own output directory: the
// rendered manifest stream ("rendered.yaml", helm template's own
// stdout) and a plain copy of the chart source itself ("chart/") —
// the second exists only because Validate's own frozen signature
// receives nothing but the already-rendered *ops.Rendered, and the
// real call path (internal/cli/ops_drift.go's render.Load) never
// carries anything Render set beyond what the cache sidecar persists;
// see .ai/phase-26/plan.md §1.1.13/§1.1.14 for the full reasoning
// behind this, and §0.4 for why no policy/scan check is ever pointed
// at chart source directly (a Helm template file is not valid YAML
// before rendering — conftest confirmed, for real, to abort outright
// on one).
package helm
