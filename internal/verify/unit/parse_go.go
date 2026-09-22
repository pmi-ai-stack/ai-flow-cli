package unit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// goTestEvent is one line of `go test -json`'s output.
type goTestEvent struct {
	Action  string `json:"Action"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
	Package string `json:"Package"`
}

// parseGo parses `go test -json`'s line-delimited event stream.
// Package-level events (Test == "") are ignored — only per-test
// run/pass/fail/skip and their accompanying output lines are
// aggregated, keyed by Test name so an interleaved multi-package run
// still groups each test's own output correctly.
func parseGo(data []byte) (Result, error) {
	type acc struct {
		status string // "pass", "fail", "skip"
		output bytes.Buffer
	}
	tests := map[string]*acc{}
	var order []string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return Result{}, fmt.Errorf("unit: parse go test -json line: %w", err)
		}
		if ev.Test == "" {
			continue
		}
		a, ok := tests[ev.Test]
		if !ok {
			a = &acc{}
			tests[ev.Test] = a
			order = append(order, ev.Test)
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			a.status = ev.Action
		case "output":
			a.output.WriteString(ev.Output)
		}
	}
	if err := scanner.Err(); err != nil {
		return Result{}, fmt.Errorf("unit: scan go test -json output: %w", err)
	}

	sort.Strings(order)
	var r Result
	for _, name := range order {
		a := tests[name]
		r.Total++
		switch a.status {
		case "pass":
			r.Passed++
		case "fail":
			r.Failed++
			r.Failures = append(r.Failures, Failure{Name: name, Message: a.output.String()})
		case "skip":
			// Counted in Total, neither Passed nor Failed — the schema
			// carries no separate skipped field, so a skipped test is
			// simply absent from both counts.
		}
	}
	return r, nil
}
