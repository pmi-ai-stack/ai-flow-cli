package kubectl

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// kubeconformReport mirrors kubeconform's own "-output json -summary"
// shape closely enough for this addon's needs. This shape is not
// recorded from a real binary (none is available in this environment —
// see .ai/phase-22/plan.md §0.8/§1.2.3) and is built from kubeconform's
// publicly documented output conventions instead — flagged provisional
// the same way phase 9's Cursor payload shapes and phase 20's doctor
// read_only heuristic already are, revisit against a real binary.
type kubeconformReport struct {
	Resources []struct {
		Filename string `json:"filename"`
		Status   string `json:"status"`
		Msg      string `json:"msg"`
	} `json:"resources"`
}

// kubeconformStatus is a resource's own validation outcome — only
// "invalid"/"error" become a Finding; "valid"/"skipped"/"empty" do
// not.
const (
	kubeconformStatusInvalid = "invalid"
	kubeconformStatusError   = "error"
)

// Validate runs kubeconform against r (a plain manifest copy — no
// prior init/build step this addon's own Render ever performs, unlike
// terraform's Validate which reuses Render's own init). kubeconform
// exits non-zero when it finds invalid resources, tolerated via
// exec.IsToolFailed exactly like tflint's own tolerance in the
// terraform addon.
func (a *Addon) Validate(ctx context.Context, r *ops.Rendered) ([]ops.Finding, error) {
	out, err := a.x.Run(ctx, "kubeconform", []string{"-output", "json", "-summary"}, exec.RunOpts{Dir: r.Dir, Timeout: a.timeout()})
	if err != nil && !exec.IsToolFailed(err) {
		return nil, err
	}

	var report kubeconformReport
	if decErr := json.Unmarshal([]byte(out.Stdout), &report); decErr != nil {
		return nil, fmt.Errorf("kubectl: decode kubeconform report: %w", decErr)
	}

	findings := make([]ops.Finding, 0, len(report.Resources))
	for _, res := range report.Resources {
		severity, ok := kubeconformSeverity(res.Status)
		if !ok {
			continue
		}
		findings = append(findings, ops.Finding{
			File:     res.Filename,
			Message:  res.Msg,
			Severity: severity,
			Tool:     "kubeconform",
		})
	}
	return findings, nil
}

func kubeconformSeverity(status string) (string, bool) {
	switch status {
	case kubeconformStatusInvalid:
		return "error", true
	case kubeconformStatusError:
		return "warning", true
	default:
		return "", false
	}
}
