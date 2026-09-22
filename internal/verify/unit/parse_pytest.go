package unit

import (
	"encoding/xml"
	"fmt"
)

// junitTestsuites is pytest's `--junitxml` report — a <testsuites>
// root wrapping one or more <testsuite>, each carrying <testcase>
// entries with an optional nested <failure> or <error>.
type junitTestsuites struct {
	Suites []junitTestsuite `xml:"testsuite"`
}

type junitTestsuite struct {
	Cases []junitTestcase `xml:"testcase"`
}

type junitTestcase struct {
	Classname string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	Failure   *junitFailure `xml:"failure"`
	Error     *junitFailure `xml:"error"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// parsePytest parses a pytest JUnit XML report. Some pytest versions
// emit a bare <testsuite> root instead of <testsuites><testsuite>;
// both are tried, in that order, so either shape parses.
func parsePytest(data []byte) (Result, error) {
	var suites junitTestsuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		return Result{}, fmt.Errorf("unit: parse pytest junitxml: %w", err)
	}
	if len(suites.Suites) == 0 {
		var single junitTestsuite
		if err := xml.Unmarshal(data, &single); err != nil {
			return Result{}, fmt.Errorf("unit: parse pytest junitxml: %w", err)
		}
		suites.Suites = []junitTestsuite{single}
	}

	var r Result
	for _, s := range suites.Suites {
		for _, c := range s.Cases {
			r.Total++
			f := c.Failure
			if f == nil {
				f = c.Error
			}
			if f == nil {
				r.Passed++
				continue
			}
			r.Failed++
			name := c.Name
			if c.Classname != "" {
				name = c.Classname + "::" + c.Name
			}
			msg := f.Message
			if msg == "" {
				msg = f.Text
			}
			r.Failures = append(r.Failures, Failure{Name: name, Message: msg})
		}
	}
	return r, nil
}
