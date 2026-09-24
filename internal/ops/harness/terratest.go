package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// runTerratest runs `go test -json ./...` in in.Dir (the harness
// file's own directory under the target's real render-cache tree —
// terraform.Harnesses reports one Terratest harness per *_test.go
// file, so in.Dir already narrows to that file's own package). Uses
// the already-allowlisted class: none "go test" row phase 6 built for
// internal/verify/unit — no new allowlist row (plan design decision
// 4). Disclosed, not solved here: a real Terratest suite typically
// shells out to terraform itself, inside this very subprocess, outside
// internal/ops/exec's own allowlist entirely — accepted by the spec's
// own scoping of e2e harnesses to a sandbox-tier environment, not a
// guarantee this function can add.
func runTerratest(ctx context.Context, x exec.Execer, in RunInput) (Result, error) {
	out, err := x.Run(ctx, "go", []string{"test", "-json", "./..."}, exec.RunOpts{Dir: in.Dir, Timeout: in.Timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return Result{}, err
	}
	res := parseGoTestJSON([]byte(out.Stdout))
	res.Raw = []byte(out.Stdout)
	return res, nil
}

// goTestEvent is one line of `go test -json`'s output — the same
// shape internal/verify/unit's own unexported parseGo reads; duplicated
// here rather than imported/exported, since internal/verify/unit's own
// Result type carries fields (Repo, Runner) this package has no use
// for and no mandate to couple to.
type goTestEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
	Output string `json:"Output"`
}

// parseGoTestJSON aggregates `go test -json`'s per-test pass/fail/skip
// actions, keyed by Test name; a failed test's accumulated Output
// (trimmed) becomes its Failures[] entry. Package-level events
// (Test == "") are ignored.
func parseGoTestJSON(data []byte) Result {
	type acc struct {
		status string
		output strings.Builder
	}
	tests := map[string]*acc{}
	var order []string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
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

	res := Result{Tool: "terratest"}
	sort.Strings(order)
	for _, name := range order {
		a := tests[name]
		res.Total++
		switch a.status {
		case "pass":
			res.Passed++
		case "skip":
			res.Skipped++
		default: // "fail", or a test whose terminal action never arrived
			res.Failed++
			res.Failures = append(res.Failures, name+": "+strings.TrimSpace(a.output.String()))
		}
	}
	return res
}
