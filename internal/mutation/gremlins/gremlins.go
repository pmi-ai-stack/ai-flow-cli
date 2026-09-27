// Package gremlins implements mutation.Tool for gremlins, the Go
// mutation-testing tool detect.Mutation names for any repository with
// a go.mod (internal/detect/mutation.go).
package gremlins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// reportFile is the fixed, relative path gremlins is asked to write
// its JSON report to — fixed rather than a generated temp name so the
// argv this package builds is deterministic and testable, and removed
// after every run so it never becomes a stray untracked file (mirrors
// internal/verify/unit's pytestReportFile).
const reportFile = ".ai-flow-gremlins-report.json"

// Tool implements mutation.Tool for gremlins.
type Tool struct{}

var _ mutation.Tool = Tool{}

// packageDirs reduces scope.Files (individual changed .go files) to
// their containing directories, deduplicated and sorted — gremlins
// unleashes on directories, not individual files.
//
// No "/..." suffix: found wrong against a real gremlins v0.6.0 binary
// while verifying phase 30 (`.ai/phase-30/implementation-log.md`).
// Unlike `go test`, gremlins' own `unleash [path]` already walks path
// recursively on its own — appending Go's own "/..." package-pattern
// suffix, as this function did before, produces either a doubled
// "path/.../..." (a subdirectory, rejected by `go list`) or "./..."
// at the repository root, which this exact gremlins version silently
// reports "No results to report" for instead of scanning anything.
// Every mutation run this package has ever attempted was therefore
// broken, at every scope, until this fix — never caught earlier
// because internal/mutation/gremlins's own tests exercised Parse
// against recorded reports and Run against a fake Execer, never a
// real binary (phase 7 shipped it against "recorded reports" by its
// own design; TestRealGremlinsRun, added in phase 30, is the first
// caller in this package's history to invoke the real tool).
//
// One real limitation this fix does not attempt to solve: gremlins'
// own `unleash` accepts exactly one positional path (confirmed
// directly: two paths in one invocation exit 2, "accepts at most 1
// arg(s)"), and that one path recurses into everything beneath it —
// so if the repository root itself is among the returned directories
// (a changed file at the module root), Run below scans the whole
// module, not just root-level files, and if a returned directory is
// an ancestor of another returned directory, mutants under the
// descendant are counted twice across their two separate invocations.
// Both are disclosed, accepted imprecisions of "changed-code scope"
// for this specific tool, not silently swallowed.
func packageDirs(files []string) []string {
	var dirs []string
	for _, f := range files {
		d := path.Dir(f)
		if d == "." {
			dirs = append(dirs, ".")
		} else {
			dirs = append(dirs, "./"+d)
		}
	}
	return strutil.DedupSorted(dirs)
}

// Run executes gremlins once per changed directory packageDirs names —
// one invocation per call, per gremlins' own single-positional-path
// contract (see packageDirs' own doc comment) — and merges the parsed
// reports. Survivor IDs are renumbered across the merge so two
// directories' own gremlins-0 entries never collide.
func (Tool) Run(ctx context.Context, repo detect.Repo, scope mutation.Scope, x exec.Execer, timeout time.Duration) (mutation.Report, error) {
	dirs := packageDirs(scope.Files)
	if len(dirs) == 0 {
		return mutation.Report{}, nil
	}

	reportPath := filepath.Join(repo.LocalPath, reportFile)
	defer func() { _ = os.Remove(reportPath) }()

	var merged mutation.Report
	for _, dir := range dirs {
		_, err := x.Run(ctx, "gremlins", []string{"unleash", "--output", reportFile, dir}, exec.RunOpts{Dir: repo.LocalPath, Timeout: timeout})
		if err != nil && !exec.IsToolFailed(err) {
			return mutation.Report{}, err
		}

		data, rerr := os.ReadFile(reportPath) //nolint:gosec // G304: reportPath is built from repo.LocalPath, an in-scope repository ai-flow already resolved
		if rerr != nil {
			return mutation.Report{}, fmt.Errorf("gremlins: read report for %s: %w", dir, rerr)
		}
		r, perr := Parse(data)
		if perr != nil {
			return mutation.Report{}, perr
		}
		merged.Total += r.Total
		merged.Killed += r.Killed
		merged.Survivors = append(merged.Survivors, r.Survivors...)
	}

	for i := range merged.Survivors {
		merged.Survivors[i].ID = fmt.Sprintf("gremlins-%d", i)
	}
	return merged, nil
}

// gremlinsReport mirrors gremlins' own real --output JSON shape —
// internal/report/internal.OutputResult in gremlins v0.6.0's own
// source (github.com/go-gremlins/gremlins, read directly from the
// module cache at $(go env GOPATH)/pkg/mod while verifying phase 30
// against a real binary): mutations are grouped per file, under
// "files[].mutations[]", each carrying only "type"/"status"/"line"/
// "column" — never the flat top-level "results[]" of
// `{mutant: {status, position: {file, line, column}}}` objects this
// struct wrongly assumed before (a shape this codebase's own tests
// never caught, since they exercised Parse only against hand-written
// fixture JSON in that wrong shape, never a real gremlins binary's own
// output, until TestRealGremlinsRun). The file name lives on the
// parent OutputFile, not on each mutation.
//
// This package also intentionally ignores OutputResult's own
// pre-aggregated mutants_total/mutants_killed/mutants_lived fields:
// gremlins' own mutants_total excludes NOT COVERED mutants, but this
// package's Total has always meant "every mutant found, covered or
// not" (see Parse's own doc comment) — a deliberate, pre-existing
// choice this fix preserves by recomputing from files[].mutations[]
// directly rather than trusting the tool's own totals.
type gremlinsReport struct {
	Files []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Status string `json:"status"`
			Line   int    `json:"line"`
		} `json:"mutations"`
	} `json:"files"`
}

// Parse decodes gremlins' JSON report. Every mutation whose status is
// not KILLED or LIVED counts toward Total but neither Killed nor a
// named Survivor (mutator.Status's own String() values, gremlins
// v0.6.0's real source: "NOT COVERED", "NOT VIABLE", "TIMED OUT",
// "SKIPPED") — a mutant gremlins never definitively resolved is
// neither evidence the suite caught it nor a named gap `mutation
// summarize`'s --equivalent flag can mark, since none of those carries
// a distinguishing id in this report shape.
func Parse(data []byte) (mutation.Report, error) {
	var report gremlinsReport
	if err := json.Unmarshal(data, &report); err != nil {
		return mutation.Report{}, fmt.Errorf("gremlins: parse report: %w", err)
	}

	out := mutation.Report{}
	for _, f := range report.Files {
		for _, m := range f.Mutations {
			out.Total++
			switch m.Status {
			case "KILLED":
				out.Killed++
			case "LIVED":
				out.Survivors = append(out.Survivors, mutation.Survivor{
					ID:   fmt.Sprintf("gremlins-%d", len(out.Survivors)),
					File: f.FileName,
					Line: m.Line,
				})
			case "NOT COVERED", "NOT VIABLE", "TIMED OUT", "SKIPPED":
				// Counted in Total, neither killed nor a named survivor —
				// see this function's own doc comment.
			default:
				return mutation.Report{}, fmt.Errorf("gremlins: parse report: unrecognized mutant status %q", m.Status)
			}
		}
	}
	return out, nil
}
