package harness

import (
	"context"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// runTextScanTool is Run's own shared body for kuttl/chainsaw (plan
// design decision 6): run tool against in.Dir pointed at the ticket's
// own kind cluster, then read its stdout for Go-test-style
// "--- PASS: <name>"/"--- FAIL: <name>"/"--- SKIP: <name>" lines — the
// one CLI convention both tools are documented to share, since neither
// is on PATH in this development environment to confirm a
// machine-readable report flag's exact shape (unlike terraform test,
// see terraformtest.go's own doc comment). When no such line is
// recognized at all, the whole harness collapses to one pass/fail
// verdict from the process exit code alone, rather than reporting a
// false 0/0/0 — flagged provisional throughout.
func runTextScanTool(ctx context.Context, x exec.Execer, tool string, args []string, in RunInput) (Result, error) {
	env := []string{"KUBECONFIG=" + in.Kubeconfig}
	out, err := x.Run(ctx, tool, args, exec.RunOpts{Dir: in.Dir, Env: env, Timeout: in.Timeout})
	failed := exec.IsToolFailed(err)
	if err != nil && !failed {
		return Result{}, err
	}

	res := parsePassFailLines(out.Stdout+"\n"+out.Stderr, tool)
	if res.Total == 0 {
		if failed {
			res = Result{Tool: tool, Total: 1, Failed: 1, Failures: []string{tool + ": exited non-zero; no per-case detail recognized in its output"}}
		} else {
			res = Result{Tool: tool, Total: 1, Passed: 1}
		}
	}
	res.Raw = []byte(out.Stdout)
	return res, nil
}

// parsePassFailLines scans text for Go-test-style result lines,
// tallying Total/Passed/Failed/Skipped and collecting every FAIL
// line's own case name into Failures[].
func parsePassFailLines(text, tool string) Result {
	res := Result{Tool: tool}
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "--- PASS:"):
			res.Total++
			res.Passed++
		case strings.HasPrefix(line, "--- FAIL:"):
			res.Total++
			res.Failed++
			res.Failures = append(res.Failures, strings.TrimSpace(strings.TrimPrefix(line, "--- FAIL:")))
		case strings.HasPrefix(line, "--- SKIP:"):
			res.Total++
			res.Skipped++
		}
	}
	return res
}
