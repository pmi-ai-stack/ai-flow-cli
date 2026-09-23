package terraform

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// tflintReport mirrors tflint's own "--format json" shape closely
// enough for this addon's needs: a flat issues[] list, each naming its
// own rule's severity, message and position. tflint's own top-level
// object also carries "errors" (its own tool-level failures); this
// addon does not read that field — a tflint invocation failure is
// already surfaced as tool_failed by exec.Runner.Run before this
// parser ever sees a byte.
type tflintReport struct {
	Issues []struct {
		Rule struct {
			Severity string `json:"severity"`
		} `json:"rule"`
		Message string `json:"message"`
		Range   struct {
			Filename string `json:"filename"`
			Start    struct {
				Line int `json:"line"`
			} `json:"start"`
		} `json:"range"`
	} `json:"issues"`
}

// Validate runs tflint against r (already initialized by Render — no
// second "terraform init" here). tflint exits non-zero when it finds
// issues, tolerated via exec.IsToolFailed exactly like
// internal/mutation/gremlins.Run already tolerates its own tool's
// findings-shaped non-zero exit.
func (a *Addon) Validate(ctx context.Context, r *ops.Rendered) ([]ops.Finding, error) {
	out, err := a.x.Run(ctx, "tflint", []string{"--format", "json"}, exec.RunOpts{Dir: r.Dir, Timeout: a.timeout()})
	if err != nil && !exec.IsToolFailed(err) {
		return nil, err
	}

	var report tflintReport
	if decErr := json.Unmarshal([]byte(out.Stdout), &report); decErr != nil {
		return nil, fmt.Errorf("terraform: decode tflint report: %w", decErr)
	}

	findings := make([]ops.Finding, 0, len(report.Issues))
	for _, iss := range report.Issues {
		findings = append(findings, ops.Finding{
			File:     iss.Range.Filename,
			Line:     iss.Range.Start.Line,
			Message:  iss.Message,
			Severity: iss.Rule.Severity,
			Tool:     "tflint",
		})
	}
	return findings, nil
}
