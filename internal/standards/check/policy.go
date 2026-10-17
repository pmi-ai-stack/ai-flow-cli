package check

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// runPolicy implements the "policy" kind: conftest, run with pack rego
// against either the target's own source directory (Check.Input ==
// "source", the default) or its already-written rendering
// (Check.Input == "render") — see target.go's targetInputDir. Unlike
// every phase-14 runner, this one is always OPS-target-scoped, never
// repository-scoped: every kind-"policy" example in the spec's own
// pack format narrows by applies_to.addons, never applies_to.languages
// (see the phase-25 plan's design decision 1) — internal/cli/checks.go
// is the only caller and never passes a zero-value RunInput.Target.
//
// conftest test's own JSON shape (per file: filename, namespace,
// failures[], warnings[], each {msg}) was confirmed against a real
// conftest binary (v1.20.2's own OPA, installed for this phase's own
// implementation via `go install
// github.com/open-policy-agent/conftest@latest`, network access
// permitting — not vendored or assumed present in every environment
// this binary ships to) run for real against the reference pack's own
// rego and a throwaway .tf/.yaml fixture: conftestFileResult's fields
// match conftest's real `--output json` output exactly.
func runPolicy(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	if in.Check.Tool != "" && in.Check.Tool != "conftest" {
		return nil, fmt.Errorf("check: policy: no runner for tool %q", in.Check.Tool)
	}

	inputDir, err := targetInputDir(in)
	if err != nil {
		return nil, err
	}

	policyDir, cleanup, err := materializeConfigDir(in)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"test", "--policy", policyDir, "--output", "json", "."}
	stdout, ok, err := runToolJSON(ctx, x, "conftest", args, inputDir, timeout)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	var report []conftestFileResult
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		return nil, fmt.Errorf("check: conftest: output does not parse: %w", err)
	}

	// warnSeverity is conftest's own "warn" rules mapped one notch
	// below the check's declared severity — conftest itself reports no
	// graded severity at all (a rule is only deny/warn), so "medium" is
	// this codebase's own fixed stand-in for "a warning, not a
	// failure," run through the same CapSeverity every graded scan tool
	// uses (the phase-25 plan's design decision 7).
	warnSeverity := CapSeverity("medium", in.Check.Severity)

	// A failure's severity is left empty (decorateFindings then defaults
	// it to the check's own declared severity — a deny rule is exactly
	// as severe as the check that names it); a warning's is set
	// explicitly, above.
	var findings []Finding
	for _, r := range report {
		for _, f := range r.Failures {
			findings = append(findings, scanFinding(in, "conftest", r.Filename, 0, f.Msg, ""))
		}
		for _, w := range r.Warnings {
			findings = append(findings, scanFinding(in, "conftest", r.Filename, 0, w.Msg, warnSeverity))
		}
	}
	return findings, nil
}

// conftestFileResult is one entry of conftest's own `--output json`
// array — one per scanned input file.
type conftestFileResult struct {
	Filename string        `json:"filename"`
	Failures []conftestMsg `json:"failures"`
	Warnings []conftestMsg `json:"warnings"`
}

type conftestMsg struct {
	Msg string `json:"msg"`
}
