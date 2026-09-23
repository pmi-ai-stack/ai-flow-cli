// Package ops holds the OPS vocabulary every later phase's addon and
// stage builds against: Addon, Capabilities, Target, Env, Rendered,
// DriftReport, Changeset, RollbackInfo, Harness, and the registry that
// enumerates every addon this binary knows about.
//
// No addon implementation lives here — terraform and kubectl arrive in
// phases 21/22, helm/kustomize in 26, github-actions/jenkins in 27.
// This package and internal/ops/exec (the allowlist) and
// internal/ops/envs (environment binding) are deliberately
// addon-independent: the exec allowlist is data, not code wired to a
// specific Go type, and DefaultRegistry starts empty in production —
// `ops addons` reporting zero rows today is correct, not a stub, until
// phase 21 registers the first real one.
package ops
