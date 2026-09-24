package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// runTerraformTest runs `terraform test -json -no-color -filter=<Filter>`
// in in.Dir (the target's own real render-cache directory, already
// terraform-init'd by Addon.Render — terraform test performs its own
// implicit init beyond that when needed, confirmed by hand-running it
// against an un-initialized fixture module in this session: it
// succeeds with no separate `terraform init` call). Tolerates
// exec.IsToolFailed (a failed run block, confirmed in this session to
// exit 1) exactly like internal/verify/unit's own Run: Output is
// parsed regardless.
func runTerraformTest(ctx context.Context, x exec.Execer, in RunInput) (Result, error) {
	args := []string{"test", "-json", "-no-color", "-filter=" + in.Filter}
	out, err := x.Run(ctx, "terraform", args, exec.RunOpts{Dir: in.Dir, Timeout: in.Timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return Result{}, err
	}
	res := parseTerraformTestJSON([]byte(out.Stdout))
	res.Raw = []byte(out.Stdout)
	// A 0/0/0 result here is a real, successful "0 run blocks matched"
	// outcome (confirmed by hand for an unmatched -filter) or a
	// malformed stream with no test_summary event at all (not observed
	// in this session's own real runs) — either way, reported as-is
	// rather than inventing a fallback verdict the way kuttl/chainsaw's
	// own text-scan parser must: terraform test's own machine-readable
	// output gives no ambiguity a fallback could resolve here.
	return res, nil
}

// tfTestEvent is one line of `terraform test -json`'s NDJSON event
// stream — the subset this parser reads, captured directly from a real
// `terraform test -json` run against a throwaway fixture module in
// this development environment (terraform v1.16.4; see the phase-24
// plan's §0), not from documentation: every field name and value here
// was observed on stdout, not guessed.
type tfTestEvent struct {
	Type        string `json:"type"`
	TestSummary *struct {
		Status  string `json:"status"`
		Passed  int    `json:"passed"`
		Failed  int    `json:"failed"`
		Errored int    `json:"errored"`
		Skipped int    `json:"skipped"`
	} `json:"test_summary"`
	Diagnostic *struct {
		Severity string `json:"severity"`
		Summary  string `json:"summary"`
		Detail   string `json:"detail"`
	} `json:"diagnostic"`
	TestRun *string `json:"@testrun"`
}

// parseTerraformTestJSON folds terraform test's own three-way
// passed/failed/errored/skipped test_summary into this package's
// Total/Passed/Failed/Skipped shape (errored folded into Failed — both
// mean the run block did not pass, and Result has no fourth bucket to
// put it in) and collects every error-severity diagnostic's own
// @testrun + detail as one Failures[] entry, in event order. A stream
// with no test_summary event at all (malformed output — not observed
// in this session's own real runs, kept as a defensive fallback the
// same way every other under-verified parser in this codebase has one)
// reports Total: 0, letting runTerraformTest's own zero-result path
// stand rather than inventing a fabricated pass/fail verdict terraform
// test's own machine-readable output gives no basis for.
func parseTerraformTestJSON(data []byte) Result {
	res := Result{Tool: "terraform-test"}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev tfTestEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // one unparseable line does not invalidate the rest
		}
		switch ev.Type {
		case "test_summary":
			if ev.TestSummary != nil {
				res.Passed = ev.TestSummary.Passed
				res.Failed = ev.TestSummary.Failed + ev.TestSummary.Errored
				res.Skipped = ev.TestSummary.Skipped
				res.Total = res.Passed + res.Failed + res.Skipped
			}
		case "diagnostic":
			if ev.Diagnostic != nil && ev.Diagnostic.Severity == "error" {
				run := ""
				if ev.TestRun != nil {
					run = *ev.TestRun
				}
				detail := ev.Diagnostic.Detail
				if detail == "" {
					detail = ev.Diagnostic.Summary
				}
				res.Failures = append(res.Failures, strings.TrimSpace(run+": "+detail))
			}
		}
	}
	return res
}
