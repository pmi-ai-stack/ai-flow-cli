// Package e2e runs and parses one repository's e2e suite — Playwright
// today (the plan's design decision 3); a second framework is a second
// branch in internal/detect.E2E plus a second parser here, not a
// signature change.
package e2e

import (
	"context"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// Failure is one failing e2e case.
type Failure struct {
	Name    string
	Message string
}

// Result is the normalized shape one e2e run produces — e2e-results.json's
// own shape, including the CaseNames/NewCases mechanism (the plan's
// design decision 4): CaseNames is every case this run saw, for the
// *next* call to diff against; NewCases is this run's names minus the
// names the caller read back from the previous e2e-results.json (empty
// when there was no previous run to compare against).
type Result struct {
	Total     int
	Passed    int
	Failed    int
	Failures  []Failure
	CaseNames []string
	NewCases  []string
}

// Run executes m's command against repo via x, appending `--grep
// caseFilter` when caseFilter is non-empty, then parses the Playwright
// JSON reporter output and computes NewCases against
// previousCaseNames. hasPrevious distinguishes "no e2e-results.json
// existed yet" (the very first call for this ticket) from "one existed
// and recorded zero cases" — an explicit bool rather than relying on
// previousCaseNames being nil vs. a non-nil empty slice, since that
// distinction is easy for a caller to lose by accident (e.g.
// json.Unmarshal into a pre-initialized slice) and this function's own
// design decision (the plan's design decision 4) — "a first run
// reports new_cases: [] — nothing to compare against yet" — depends on
// getting it right. Mirrors internal/verify/unit.Run's tolerance of a
// tool_failed error (a suite with failing cases still exits non-zero,
// and Output is still populated) — see that package's Run for the full
// reasoning.
func Run(ctx context.Context, repo detect.Repo, m detect.E2EMatch, caseFilter string, previousCaseNames []string, hasPrevious bool, x exec.Execer, timeout time.Duration) (Result, error) {
	args := append([]string{}, m.Args...)
	if caseFilter != "" {
		args = append(args, "--grep", caseFilter)
	}

	out, err := x.Run(ctx, m.Command, args, exec.RunOpts{Dir: repo.LocalPath, Timeout: timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return Result{}, err
	}

	result, perr := parsePlaywright([]byte(out.Stdout))
	if perr != nil {
		return Result{}, perr
	}

	result.NewCases = newCases(result.CaseNames, previousCaseNames, hasPrevious)
	return result, nil
}

// newCases is the plan's design decision 4 as a pure function of two
// name lists: every name in current not in previous, or none at all
// when hasPrevious is false ("nothing to compare against yet" — see
// Run's own doc comment on why that is a separate bool rather than an
// empty-vs-nil previous slice).
func newCases(current, previous []string, hasPrevious bool) []string {
	if !hasPrevious {
		return []string{}
	}
	seen := make(map[string]bool, len(previous))
	for _, n := range previous {
		seen[n] = true
	}
	var out []string
	for _, n := range current {
		if !seen[n] {
			out = append(out, n)
		}
	}
	result := strutil.DedupSorted(out)
	if result == nil {
		result = []string{}
	}
	return result
}
