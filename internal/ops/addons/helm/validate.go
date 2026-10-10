package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// helmLintLine matches one "helm lint" output line — "[LEVEL] location:
// message" — verified against a real "helm lint" binary (v3.16.2),
// both a clean and a broken chart, see .ai/phase-26/plan.md §0.1. INFO
// is advisory only and produces no finding (the same "some statuses
// produce nothing" shape kubeconform's own valid/skipped/empty already
// establishes).
var helmLintLine = regexp.MustCompile(`^\[(ERROR|WARNING|INFO)\]\s+([^:]+):\s*(.+)$`)

// kubeconformReport mirrors kubectl's own — provisional, kubeconform
// remains absent in this environment (.ai/phase-26/plan.md §0.1).
type kubeconformReport struct {
	Resources []struct {
		Filename string `json:"filename"`
		Status   string `json:"status"`
		Msg      string `json:"msg"`
	} `json:"resources"`
}

const (
	kubeconformStatusInvalid = "invalid"
	kubeconformStatusError   = "error"
)

// Validate runs both tools the capability matrix's own helm row names:
// "helm lint" against the chart source Render copied into
// <r.Dir>/chart (see render.go's own doc comment for why), and
// kubeconform against r.Dir itself (the rendering) — identical
// invocation to kubectl's/kustomize's own.
func (a *Addon) Validate(ctx context.Context, r *ops.Rendered) ([]ops.Finding, error) {
	lintFindings, err := a.runHelmLint(ctx, r.Dir)
	if err != nil {
		return nil, err
	}

	kubeconformFindings, err := a.runKubeconform(ctx, r.Dir)
	if err != nil {
		return nil, err
	}

	return append(lintFindings, kubeconformFindings...), nil
}

func (a *Addon) runHelmLint(ctx context.Context, renderedDir string) ([]ops.Finding, error) {
	chartDir := filepath.Join(renderedDir, chartCopyDirName)
	args := []string{"lint", chartDir}
	if vf, ok := findValuesFile(chartDir); ok {
		args = append(args, "-f", vf)
	}
	out, err := a.x.Run(ctx, "helm", args, exec.RunOpts{Timeout: a.timeout()})
	if err != nil && !exec.IsToolFailed(err) {
		return nil, err
	}

	var findings []ops.Finding
	for line := range strings.SplitSeq(out.Stdout, "\n") {
		m := helmLintLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		severity, ok := helmLintSeverity(m[1])
		if !ok {
			continue
		}
		findings = append(findings, ops.Finding{
			File:     m[2],
			Message:  m[3],
			Severity: severity,
			Tool:     "helm lint",
		})
	}
	return findings, nil
}

func helmLintSeverity(level string) (string, bool) {
	switch level {
	case "ERROR":
		return "error", true
	case "WARNING":
		return "warning", true
	default: // INFO
		return "", false
	}
}

func (a *Addon) runKubeconform(ctx context.Context, dir string) ([]ops.Finding, error) {
	out, err := a.x.Run(ctx, "kubeconform", []string{"-output", "json", "-summary"}, exec.RunOpts{Dir: dir, Timeout: a.timeout()})
	if err != nil && !exec.IsToolFailed(err) {
		return nil, err
	}

	var report kubeconformReport
	if decErr := json.Unmarshal([]byte(out.Stdout), &report); decErr != nil {
		return nil, fmt.Errorf("helm: decode kubeconform report: %w", decErr)
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
