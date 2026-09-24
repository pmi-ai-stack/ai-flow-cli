package githubactions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// actionlintFinding mirrors actionlint's own
// "-format '{{json .}}'" shape exactly — verified for real against a
// real actionlint v1.7.12 binary (the phase-27 plan's own §0.1), both
// a clean workflow (empty array) and a deliberately broken one.
type actionlintFinding struct {
	Message  string `json:"message"`
	Filepath string `json:"filepath"`
	Line     int    `json:"line"`
}

// Validate runs actionlint against r's own rendered workflow file.
// actionlint exits non-zero when it finds issues, tolerated via
// exec.IsToolFailed exactly like every other addon's own Validate.
// Every finding is reported at Severity "error" — actionlint's own
// JSON carries no graded severity of its own to preserve (unlike
// tflint's per-rule severity or kubeconform's status), every finding
// it reports is a real problem.
func (a *Addon) Validate(ctx context.Context, r *ops.Rendered) ([]ops.Finding, error) {
	out, err := a.x.Run(ctx, "actionlint", []string{"-format", "{{json .}}", RenderedFile}, exec.RunOpts{Dir: r.Dir, Timeout: a.timeout()})
	if err != nil && !exec.IsToolFailed(err) {
		return nil, err
	}

	var report []actionlintFinding
	if decErr := json.Unmarshal([]byte(out.Stdout), &report); decErr != nil {
		return nil, fmt.Errorf("github-actions: decode actionlint report: %w", decErr)
	}

	findings := make([]ops.Finding, 0, len(report))
	for _, f := range report {
		findings = append(findings, ops.Finding{
			File: f.Filepath, Line: f.Line, Message: f.Message,
			Severity: "error", Tool: "actionlint",
		})
	}
	return findings, nil
}
