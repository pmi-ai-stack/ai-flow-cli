// Package stryker implements mutation.Tool for Stryker, the
// JS/TS mutation-testing tool detect.Mutation names for any
// repository whose package.json depends on @stryker-mutator/core
// (internal/detect/mutation.go).
package stryker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// reportPath is Stryker's own default JSON-reporter output path
// (relative to the repository root) — this package asks for nothing
// else, so no --reporters-json-file flag is needed.
const reportPath = "reports/mutation/mutation-report.json"

// Tool implements mutation.Tool for Stryker.
type Tool struct{}

var _ mutation.Tool = Tool{}

// Run executes `npx stryker run --mutate <scope.Files> --reporters
// json` and parses the JSON report it wrote.
func (Tool) Run(ctx context.Context, repo detect.Repo, scope mutation.Scope, x exec.Execer, timeout time.Duration) (mutation.Report, error) {
	if len(scope.Files) == 0 {
		return mutation.Report{}, nil
	}

	args := []string{"stryker", "run", "--mutate", strings.Join(scope.Files, ","), "--reporters", "json"}
	full := filepath.Join(repo.LocalPath, reportPath)
	defer func() { _ = os.Remove(full) }()

	_, err := x.Run(ctx, "npx", args, exec.RunOpts{Dir: repo.LocalPath, Timeout: timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return mutation.Report{}, err
	}

	data, rerr := os.ReadFile(full) //nolint:gosec // G304: full is built from repo.LocalPath, an in-scope repository ai-flow already resolved
	if rerr != nil {
		return mutation.Report{}, fmt.Errorf("stryker: read report: %w", rerr)
	}
	return Parse(data)
}

// strykerReport mirrors the mutation-testing-report-schema shape
// Stryker's JSON reporter writes: files, each with mutants carrying a
// status and a source location.
type strykerReport struct {
	Files map[string]struct {
		Mutants []struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Location struct {
				Start struct {
					Line int `json:"line"`
				} `json:"start"`
			} `json:"location"`
		} `json:"mutants"`
	} `json:"files"`
}

// Parse decodes Stryker's JSON report. "Killed" and "Timeout" both
// count as caught (a mutant that hangs the suite is not a test gap);
// "Survived" and "NoCoverage" both count as survivors — NoCoverage
// still carries a real mutant id a --equivalent call can name, unlike
// gremlins' NOT_COVERED status which carries no id in that tool's own
// report shape. "Ignored" and "CompileError" are excluded from Total
// entirely: Stryker's own scoring formula already excludes both.
func Parse(data []byte) (mutation.Report, error) {
	var report strykerReport
	if err := json.Unmarshal(data, &report); err != nil {
		return mutation.Report{}, fmt.Errorf("stryker: parse report: %w", err)
	}

	files := make([]string, 0, len(report.Files))
	for file := range report.Files {
		files = append(files, file)
	}
	sort.Strings(files) // report.Files is a map — sort for byte-stable Survivors order

	out := mutation.Report{}
	for _, file := range files {
		f := report.Files[file]
		for _, m := range f.Mutants {
			switch m.Status {
			case "Killed", "Timeout":
				out.Total++
				out.Killed++
			case "Survived", "NoCoverage":
				out.Total++
				out.Survivors = append(out.Survivors, mutation.Survivor{ID: m.ID, File: file, Line: m.Location.Start.Line})
			case "Ignored", "CompileError":
				// Excluded from Total — Stryker's own score formula
				// excludes both.
			default:
				return mutation.Report{}, fmt.Errorf("stryker: parse report: unrecognized mutant status %q", m.Status)
			}
		}
	}
	return out, nil
}
