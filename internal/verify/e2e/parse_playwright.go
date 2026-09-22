package e2e

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// playwrightReport is `playwright test --reporter=json`'s output —
// suites nest arbitrarily (a describe block is a nested suite), so
// this shape mirrors that recursively.
type playwrightReport struct {
	Suites []playwrightSuite `json:"suites"`
}

type playwrightSuite struct {
	Title  string            `json:"title"`
	Suites []playwrightSuite `json:"suites"`
	Specs  []playwrightSpec  `json:"specs"`
}

type playwrightSpec struct {
	Title string           `json:"title"`
	Tests []playwrightTest `json:"tests"`
}

type playwrightTest struct {
	Results []playwrightResult `json:"results"`
}

type playwrightResult struct {
	Status string `json:"status"` // "passed", "failed", "timedOut", "skipped"
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// parsePlaywright parses the JSON reporter's report. A case's name is
// its suite path joined by " > " (matching how Playwright itself
// prints a failing case's location); its verdict is its first result's
// status — Playwright retries fill Results with more than one entry,
// but the first is what a single, non-retried CI run always produces.
func parsePlaywright(data []byte) (Result, error) {
	var report playwrightReport
	if err := json.Unmarshal(data, &report); err != nil {
		return Result{}, fmt.Errorf("e2e: parse playwright --reporter=json: %w", err)
	}

	var r Result
	var names []string
	for _, s := range report.Suites {
		walkSuite(s, "", &r, &names)
	}
	r.CaseNames = strutil.DedupSorted(names)
	if r.CaseNames == nil {
		r.CaseNames = []string{}
	}
	sort.Slice(r.Failures, func(i, j int) bool { return r.Failures[i].Name < r.Failures[j].Name })
	return r, nil
}

func walkSuite(s playwrightSuite, prefix string, r *Result, names *[]string) {
	path := s.Title
	if prefix != "" {
		path = prefix + " > " + s.Title
	}
	for _, spec := range s.Specs {
		name := path
		if spec.Title != "" {
			name = path + " > " + spec.Title
		}
		*names = append(*names, name)
		r.Total++
		status, msg := specVerdict(spec)
		switch status {
		case "passed":
			r.Passed++
		case "failed", "timedOut":
			r.Failed++
			r.Failures = append(r.Failures, Failure{Name: name, Message: msg})
		case "skipped":
			// Counted in Total, neither Passed nor Failed.
		}
	}
	for _, nested := range s.Suites {
		walkSuite(nested, path, r, names)
	}
}

// specVerdict returns spec's first test's first result's status and,
// when it failed, that result's error message.
func specVerdict(spec playwrightSpec) (status, message string) {
	if len(spec.Tests) == 0 || len(spec.Tests[0].Results) == 0 {
		return "skipped", ""
	}
	res := spec.Tests[0].Results[0]
	msg := ""
	if res.Error != nil {
		msg = res.Error.Message
	}
	return res.Status, msg
}
