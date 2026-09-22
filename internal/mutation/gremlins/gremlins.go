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
// their containing packages, deduplicated and sorted — gremlins
// unleashes on package paths, not individual files.
func packageDirs(files []string) []string {
	var dirs []string
	for _, f := range files {
		d := path.Dir(f)
		if d == "." {
			d = "./..."
		} else {
			d = "./" + d + "/..."
		}
		dirs = append(dirs, d)
	}
	return strutil.DedupSorted(dirs)
}

// Run executes gremlins over scope's changed packages, then parses the
// JSON report it wrote.
func (Tool) Run(ctx context.Context, repo detect.Repo, scope mutation.Scope, x exec.Execer, timeout time.Duration) (mutation.Report, error) {
	dirs := packageDirs(scope.Files)
	if len(dirs) == 0 {
		return mutation.Report{}, nil
	}

	args := append([]string{"unleash", "--output", reportFile}, dirs...)
	reportPath := filepath.Join(repo.LocalPath, reportFile)
	defer func() { _ = os.Remove(reportPath) }()

	_, err := x.Run(ctx, "gremlins", args, exec.RunOpts{Dir: repo.LocalPath, Timeout: timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return mutation.Report{}, err
	}

	data, rerr := os.ReadFile(reportPath) //nolint:gosec // G304: reportPath is built from repo.LocalPath, an in-scope repository ai-flow already resolved
	if rerr != nil {
		return mutation.Report{}, fmt.Errorf("gremlins: read report: %w", rerr)
	}
	return Parse(data)
}

// gremlinsReport mirrors gremlins' own --output JSON shape: a flat
// results[] list, each entry naming its mutant's status and position.
type gremlinsReport struct {
	Results []struct {
		Mutant struct {
			Status   string `json:"status"`
			Position struct {
				File   string `json:"file"`
				Line   int    `json:"line"`
				Column int    `json:"column"`
			} `json:"position"`
		} `json:"mutant"`
	} `json:"results"`
}

// Parse decodes gremlins' JSON report. Every result whose status is
// not KILLED, NOT_COVERED or TIMED_OUT counts as a survivor (LIVED is
// gremlins' own name for "survived"); NOT_COVERED and TIMED_OUT count
// toward Total but not Killed or Survivors — a mutant gremlins never
// even exercised is neither evidence the suite caught it nor a named
// gap `mutation summarize`'s --equivalent flag can mark, since it
// carries no distinguishing id in this report shape.
func Parse(data []byte) (mutation.Report, error) {
	var report gremlinsReport
	if err := json.Unmarshal(data, &report); err != nil {
		return mutation.Report{}, fmt.Errorf("gremlins: parse report: %w", err)
	}

	out := mutation.Report{}
	for i, r := range report.Results {
		out.Total++
		switch r.Mutant.Status {
		case "KILLED":
			out.Killed++
		case "LIVED":
			out.Survivors = append(out.Survivors, mutation.Survivor{
				ID:   fmt.Sprintf("gremlins-%d", i),
				File: r.Mutant.Position.File,
				Line: r.Mutant.Position.Line,
			})
		case "NOT_COVERED", "TIMED_OUT":
			// Counted in Total, neither killed nor a named survivor —
			// see this function's own doc comment.
		default:
			return mutation.Report{}, fmt.Errorf("gremlins: parse report: unrecognized mutant status %q", r.Mutant.Status)
		}
	}
	return out, nil
}
