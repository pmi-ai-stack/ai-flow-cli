package unit

import (
	"encoding/json"
	"fmt"
	"strings"
)

// jestReport is the subset of `jest --json`'s single-object output
// this package reads.
type jestReport struct {
	NumTotalTests  int `json:"numTotalTests"`
	NumPassedTests int `json:"numPassedTests"`
	NumFailedTests int `json:"numFailedTests"`
	TestResults    []struct {
		AssertionResults []struct {
			FullName        string   `json:"fullName"`
			Title           string   `json:"title"`
			Status          string   `json:"status"`
			FailureMessages []string `json:"failureMessages"`
		} `json:"assertionResults"`
	} `json:"testResults"`
}

// parseJest parses jest's `--json` report.
func parseJest(data []byte) (Result, error) {
	var report jestReport
	if err := json.Unmarshal(data, &report); err != nil {
		return Result{}, fmt.Errorf("unit: parse jest --json: %w", err)
	}

	r := Result{
		Total:  report.NumTotalTests,
		Passed: report.NumPassedTests,
		Failed: report.NumFailedTests,
	}
	for _, tr := range report.TestResults {
		for _, a := range tr.AssertionResults {
			if a.Status != "failed" {
				continue
			}
			name := a.FullName
			if name == "" {
				name = a.Title
			}
			r.Failures = append(r.Failures, Failure{Name: name, Message: strings.Join(a.FailureMessages, "\n")})
		}
	}
	return r, nil
}
