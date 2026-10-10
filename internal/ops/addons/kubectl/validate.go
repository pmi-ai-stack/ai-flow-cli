package kubectl

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// kubeconformReport is the subset of kubeconform's "-output json
// -summary" report this addon reads, checked against real kubeconform
// v0.8.0 output in phase 58 (testdata/kubeconform-report.json): only
// invalid and error resources are listed, with statuses
// "statusInvalid" and "statusError".
type kubeconformReport struct {
	Resources []struct {
		Filename string `json:"filename"`
		Status   string `json:"status"`
		Msg      string `json:"msg"`
	} `json:"resources"`
}

// kubeconformStatus is a resource's own validation outcome — only
// invalid and error become a Finding; valid, skipped and empty do not.
const (
	kubeconformStatusInvalid = "statusInvalid"
	kubeconformStatusError   = "statusError"
)

// Validate runs kubeconform against r (a plain manifest copy — no
// prior init/build step this addon's own Render ever performs, unlike
// terraform's Validate which reuses Render's own init). kubeconform
// exits non-zero when it finds invalid resources, tolerated via
// exec.IsToolFailed exactly like tflint's own tolerance in the
// terraform addon.
func (a *Addon) Validate(ctx context.Context, r *ops.Rendered) ([]ops.Finding, error) {
	// "." is the rendering itself: with no path argument kubeconform
	// reads stdin, finds nothing and exits 1 with no report.
	out, err := a.x.Run(ctx, "kubeconform", []string{"-output", "json", "-summary", "."}, exec.RunOpts{Dir: r.Dir, Timeout: a.timeout()})
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
