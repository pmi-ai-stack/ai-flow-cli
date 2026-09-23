// Package terraform implements ops.Addon for Terraform root modules —
// the spec's own capability-matrix row: Detect by a top-level
// "terraform {}" block; Render "init -backend=false, validate";
// Validate "tflint"; Drift "plan -detailed-exitcode" from the base
// rendering; DryRun "plan -out" -> "show -json" -> a normalized
// Changeset; Rollback a text-only reverse-plan instruction plus a
// destroy/replace address list; Harnesses ".tftest.hcl" detection
// (running them is phase 24's internal/ops/harness, not this
// package). See .ai/phase-21/plan.md for the design decisions behind
// every method — in particular why Render never marks a value
// sensitive (§0.5), why env.Name == "" is always
// ops.ErrLiveCredentialsMissing here (design decision 2), and why
// Action.Protected is always false from this addon (design decision
// 3).
package terraform
