// Package risk implements the spec's own risk-scoring formula: integer
// per-mille arithmetic (never float, past config parsing — see
// PerMille), the protected/sensitive flags formula, and blast-radius
// aggregation. Every function here is pure: no subprocess, no
// filesystem, no config mutation — internal/cli/ops_impact.go is the
// one caller that reads dryrun-results.json/render-drift-results.json,
// calls this package, and writes impact-results.json.
package risk
