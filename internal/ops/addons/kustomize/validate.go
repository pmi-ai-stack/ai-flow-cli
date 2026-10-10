package kustomize

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// kubeconformReport mirrors kubectl's own — kubeconform's report shape
// is addon-independent, checked against a real binary in phase 58
// (internal/ops/addons/kubectl/testdata/kubeconform-report.json).
type kubeconformReport struct {
	Resources []struct {
		Filename string `json:"filename"`
		Status   string `json:"status"`
		Msg      string `json:"msg"`
	} `json:"resources"`
}

const (
	kubeconformStatusInvalid = "statusInvalid"
	kubeconformStatusError   = "statusError"
)

// Validate runs kubeconform against r (the rendering itself — a
// kustomize target has no separate lint step of its own the way a
// Helm chart does, so kubeconform is the entire Validate cell here,
// per the capability matrix).
func (a *Addon) Validate(ctx context.Context, r *ops.Rendered) ([]ops.Finding, error) {
	// The rendering's one file; with no path argument kubeconform reads
	// stdin, finds nothing and exits 1 with no report.
	out, err := a.x.Run(ctx, "kubeconform", []string{"-output", "json", "-summary", renderedFileName}, exec.RunOpts{Dir: r.Dir, Timeout: a.timeout()})
	if err != nil && !exec.IsToolFailed(err) {
		return nil, err
	}

	var report kubeconformReport
	if decErr := json.Unmarshal([]byte(out.Stdout), &report); decErr != nil {
		return nil, fmt.Errorf("kustomize: decode kubeconform report: %w", decErr)
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
