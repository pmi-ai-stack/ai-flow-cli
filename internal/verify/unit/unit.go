// Package unit runs and parses one repository's unit test suite —
// internal/verify.TestRunner's shape for the "unit" side, in
// implementation-plan.md's own words, applied concretely: Run
// dispatches to the exec.Execer with the argv detect.Runners already
// built, then hands the captured output to the one parser detect's
// RunnerMatch.Runner names.
package unit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Failure is one failing (or errored) test case.
type Failure struct {
	Name    string
	Message string
}

// Result is the normalized shape every runner's parser produces —
// test-results.json's per-repository entry, before internal/cli adds
// the Repo/Runner fields from the RunnerMatch that produced it.
type Result struct {
	Repo     string
	Runner   string
	Total    int
	Passed   int
	Failed   int
	Failures []Failure
}

// pytestReportFile is the fixed, relative report path pytest is asked
// to write its JUnit XML to (relative to the repository's working
// directory, i.e. opts.Dir) — fixed rather than a fresh temp name each
// call so the argv Run builds is deterministic and byte-for-byte
// testable against a scripted exec.Execer, and removed after every run
// (success or failure) so it never becomes a stray untracked file a
// later `git status`/`git diff` would see.
const pytestReportFile = ".ai-flow-pytest-report.xml"

// Run executes m's command against repo via x, then parses the result
// with the parser m.Runner names. A non-zero exit from the test tool
// itself (envelope.Error{Code: "tool_failed"}) is not propagated as an
// error — that is what a suite with failing tests looks like, and
// Output is still populated (see exec.Runner.Run's own doc comment),
// so parsing proceeds exactly as it would on a clean exit. Any other
// error (most notably exec_not_allowed, meaning the tool never ran at
// all) is propagated unchanged — there is nothing to parse.
func Run(ctx context.Context, repo detect.Repo, m detect.RunnerMatch, x exec.Execer, timeout time.Duration) (Result, error) {
	args := m.Args
	if m.Runner == "pytest" {
		args = append(append([]string{}, args...), "--junitxml="+pytestReportFile)
		reportPath := filepath.Join(repo.LocalPath, pytestReportFile)
		defer func() { _ = os.Remove(reportPath) }()
	}

	out, err := x.Run(ctx, m.Command, args, exec.RunOpts{Dir: repo.LocalPath, Timeout: timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return Result{}, err
	}

	var data []byte
	if m.Runner == "pytest" {
		data, err = os.ReadFile(filepath.Join(repo.LocalPath, pytestReportFile))
		if err != nil {
			return Result{}, fmt.Errorf("unit: read pytest report: %w", err)
		}
	} else {
		data = []byte(out.Stdout)
	}

	result, perr := parse(m.Runner, data)
	if perr != nil {
		return Result{}, perr
	}
	result.Repo = repo.Name
	result.Runner = m.Runner
	return result, nil
}

// parse dispatches to the one parser runner names.
func parse(runner string, data []byte) (Result, error) {
	switch runner {
	case "go":
		return parseGo(data)
	case "jest":
		return parseJest(data)
	case "pytest":
		return parsePytest(data)
	default:
		return Result{}, fmt.Errorf("unit: no parser for runner %q", runner)
	}
}
