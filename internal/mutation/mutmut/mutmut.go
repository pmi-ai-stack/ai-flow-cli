// Package mutmut implements mutation.Tool for mutmut, the Python
// mutation-testing tool detect.Mutation names for any repository with
// a pyproject.toml or setup.py (internal/detect/mutation.go).
package mutmut

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Tool implements mutation.Tool for mutmut.
type Tool struct{}

var _ mutation.Tool = Tool{}

// Run executes `mutmut run --paths-to-mutate <scope.Files>` (mutmut's
// own exit code reflects whether any mutant survived, tolerated the
// same way internal/verify/unit tolerates a failing suite's exit
// code), then `mutmut junitxml`, whose stdout is mutmut's own
// JUnit-shaped report of every surviving mutant as a failed test case.
func (Tool) Run(ctx context.Context, repo detect.Repo, scope mutation.Scope, x exec.Execer, timeout time.Duration) (mutation.Report, error) {
	if len(scope.Files) == 0 {
		return mutation.Report{}, nil
	}

	runArgs := append([]string{"run", "--paths-to-mutate"}, strings.Join(scope.Files, ","))
	if _, err := x.Run(ctx, "mutmut", runArgs, exec.RunOpts{Dir: repo.LocalPath, Timeout: timeout}); err != nil && !exec.IsToolFailed(err) {
		return mutation.Report{}, err
	}

	out, err := x.Run(ctx, "mutmut", []string{"junitxml"}, exec.RunOpts{Dir: repo.LocalPath, Timeout: timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return mutation.Report{}, err
	}
	return Parse([]byte(out.Stdout))
}

// junitTestsuite is the one <testsuite> mutmut junitxml writes: tests
// is the total mutant count it considered, and each <testcase> with a
// <failure> child is one surviving mutant (mutmut's own convention —
// a "failed test" is a mutant the suite did not kill). The testcase
// name carries mutmut's own mutant id.
type junitTestsuite struct {
	Tests int `xml:"tests,attr"`
	Cases []struct {
		Name      string `xml:"name,attr"`
		ClassName string `xml:"classname,attr"`
		Line      int    `xml:"line,attr"`
		Failure   *struct {
			Message string `xml:",chardata"`
		} `xml:"failure"`
	} `xml:"testcase"`
}

// Parse decodes mutmut junitxml's output. Every <testcase> without a
// <failure> child is a killed mutant; every one with a <failure> is a
// survivor, named by its own id (the plan's design decision 8).
func Parse(data []byte) (mutation.Report, error) {
	var suite junitTestsuite
	if err := xml.Unmarshal(data, &suite); err != nil {
		return mutation.Report{}, fmt.Errorf("mutmut: parse junitxml: %w", err)
	}

	out := mutation.Report{Total: suite.Tests}
	for _, c := range suite.Cases {
		if c.Failure == nil {
			out.Killed++
			continue
		}
		out.Survivors = append(out.Survivors, mutation.Survivor{ID: c.Name, File: c.ClassName, Line: c.Line})
	}
	return out, nil
}
