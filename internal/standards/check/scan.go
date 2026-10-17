package check

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// runScan implements the "scan" kind: checkov, trivy config, kyverno
// apply, zizmor (phase 27) — scanners with their own graded severity,
// mapped through CapSeverity and capped at the check's own declared
// severity (the spec's own "scan" table row: "tool; severity mapped" —
// unlike "policy", which has no graded severity of its own to map).
// Always OPS-target-scoped, the identical reasoning runPolicy's own
// doc comment states. Dispatches on Check.Tool, mirroring lint.go's
// own runLint shape.
//
// checkov/trivy/kyverno's own output shapes are not verified against a
// real binary in this development environment — flagged provisional
// in each helper's own doc comment; zizmor's own shape (runZizmor,
// below) is the first in this file verified for real (phase-27 plan
// §0.1).
func runScan(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	switch in.Check.Tool {
	case "checkov":
		return runCheckov(ctx, in, x, timeout)
	case "trivy":
		return runTrivyConfig(ctx, in, x, timeout)
	case "kyverno":
		return runKyvernoApply(ctx, in, x, timeout)
	case "zizmor":
		return runZizmor(ctx, in, x, timeout)
	default:
		return nil, fmt.Errorf("check: scan: no runner for tool %q", in.Check.Tool)
	}
}

func scanFinding(in RunInput, tool, file string, line int, message, severity string) Finding {
	return Finding{
		Tool: tool, Repo: in.Target.Repo, Target: in.Target.Key,
		File: file, Line: line, Message: message, Severity: severity, Status: "open",
	}
}

// checkovReport is checkov's own `--output json` shape, trimmed.
// checkov's own documented behaviour differs by how many frameworks it
// detects in the scanned directory: a single object when exactly one
// framework applies, an array of one-per-framework objects otherwise —
// decodeCheckovReports tolerates both, the same posture
// internal/ops/addons/kubectl/normalize.go already established for
// kubectl's own two differing JSON conventions.
type checkovReport struct {
	Results checkovResults `json:"results"`
}

type checkovResults struct {
	FailedChecks []checkovCheck `json:"failed_checks"`
}

type checkovCheck struct {
	CheckID       string `json:"check_id"`
	CheckName     string `json:"check_name"`
	Severity      string `json:"severity"`
	FilePath      string `json:"file_path"`
	FileLineRange []int  `json:"file_line_range"`
	Resource      string `json:"resource"`
}

func runCheckov(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	dir, err := targetInputDir(in)
	if err != nil {
		return nil, err
	}
	configPath, cleanup, err := materializeConfig(in)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"--directory", ".", "--output", "json", "--compact", "--quiet"}
	if configPath != "" {
		args = append(args, "--config-file", configPath)
	}
	stdout, ok, err := runToolJSON(ctx, x, "checkov", args, dir, timeout)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	reports, err := decodeCheckovReports(stdout)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, r := range reports {
		for _, c := range r.Results.FailedChecks {
			line := 0
			if len(c.FileLineRange) > 0 {
				line = c.FileLineRange[0]
			}
			sev := CapSeverity(strings.ToLower(c.Severity), in.Check.Severity)
			msg := fmt.Sprintf("[%s] %s", c.CheckID, c.CheckName)
			if c.Resource != "" {
				msg = fmt.Sprintf("%s (%s)", msg, c.Resource)
			}
			findings = append(findings, scanFinding(in, "checkov", strings.TrimPrefix(c.FilePath, "/"), line, msg, sev))
		}
	}
	return findings, nil
}

func decodeCheckovReports(stdout string) ([]checkovReport, error) {
	var arr []checkovReport
	if err := json.Unmarshal([]byte(stdout), &arr); err == nil {
		return arr, nil
	}
	var one checkovReport
	if err := json.Unmarshal([]byte(stdout), &one); err != nil {
		return nil, fmt.Errorf("check: checkov: output does not parse: %w", err)
	}
	return []checkovReport{one}, nil
}

// trivyConfigReport is `trivy config --format json`'s own shape,
// trimmed.
type trivyConfigReport struct {
	Results []trivyResult `json:"Results"`
}

type trivyResult struct {
	Target            string           `json:"Target"`
	Misconfigurations []trivyMisconfig `json:"Misconfigurations"`
}

type trivyMisconfig struct {
	ID            string `json:"ID"`
	Title         string `json:"Title"`
	Message       string `json:"Message"`
	Severity      string `json:"Severity"`
	CauseMetadata struct {
		StartLine int `json:"StartLine"`
	} `json:"CauseMetadata"`
}

func runTrivyConfig(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	dir, err := targetInputDir(in)
	if err != nil {
		return nil, err
	}
	configPath, cleanup, err := materializeConfig(in)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"config", "--format", "json"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, ".")
	stdout, ok, err := runToolJSON(ctx, x, "trivy", args, dir, timeout)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	var report trivyConfigReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		return nil, fmt.Errorf("check: trivy: output does not parse: %w", err)
	}

	var findings []Finding
	for _, r := range report.Results {
		for _, m := range r.Misconfigurations {
			sev := CapSeverity(strings.ToLower(m.Severity), in.Check.Severity)
			msg := fmt.Sprintf("[%s] %s: %s", m.ID, m.Title, m.Message)
			findings = append(findings, scanFinding(in, "trivy", r.Target, m.CauseMetadata.StartLine, msg, sev))
		}
	}
	return findings, nil
}

// kyvernoPolicyReport is the Kubernetes PolicyReport shape
// (wgpolicyk8s.io/v1alpha2) `kyverno apply --policy-report -o json`
// emits — distinct from kyverno apply's own default human-readable
// table, which is not machine-parseable at all.
type kyvernoPolicyReport struct {
	Results []kyvernoResult `json:"results"`
}

type kyvernoResult struct {
	Policy    string             `json:"policy"`
	Rule      string             `json:"rule"`
	Message   string             `json:"message"`
	Result    string             `json:"result"` // pass|fail|warn|error|skip
	Severity  string             `json:"severity"`
	Resources []kyvernoResultRef `json:"resources"`
}

type kyvernoResultRef struct {
	Name string `json:"name"`
}

func runKyvernoApply(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	if in.Check.Config == "" {
		return nil, fmt.Errorf("check: %s: kyverno apply needs a policy directory (Config), none set", in.Check.ID)
	}
	dir, err := targetInputDir(in)
	if err != nil {
		return nil, err
	}

	policyDir, cleanup, err := materializeConfigDir(in)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"apply", policyDir, "--resource", ".", "--policy-report", "-o", "json"}
	stdout, ok, err := runToolJSON(ctx, x, "kyverno", args, dir, timeout)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	var report kyvernoPolicyReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		return nil, fmt.Errorf("check: kyverno: output does not parse: %w", err)
	}

	var findings []Finding
	for _, r := range report.Results {
		if r.Result != "fail" && r.Result != "error" && r.Result != "warn" {
			continue
		}
		sev := CapSeverity(strings.ToLower(r.Severity), in.Check.Severity)
		msg := fmt.Sprintf("[%s/%s] %s", r.Policy, r.Rule, r.Message)
		if len(r.Resources) == 0 {
			findings = append(findings, scanFinding(in, "kyverno", "", 0, msg, sev))
			continue
		}
		for _, res := range r.Resources {
			findings = append(findings, scanFinding(in, "kyverno", res.Name, 0, msg, sev))
		}
	}
	return findings, nil
}

// zizmorFinding mirrors zizmor's own "--format json" shape closely
// enough for this addon's needs — verified for real against a real
// zizmor v1.30.1 binary (the phase-27 plan's own §0.1), including its
// own capitalized severity strings ("Medium", "High" confirmed for
// real) and its 0-indexed location row.
type zizmorFinding struct {
	Ident          string `json:"ident"`
	Desc           string `json:"desc"`
	Determinations struct {
		Severity string `json:"severity"`
	} `json:"determinations"`
	Locations []struct {
		Symbolic struct {
			Key struct {
				Local struct {
					VerbatimPath string `json:"verbatim_path"`
				} `json:"Local"`
			} `json:"key"`
		} `json:"symbolic"`
		Concrete struct {
			Location struct {
				StartPoint struct {
					Row int `json:"row"`
				} `json:"start_point"`
			} `json:"location"`
		} `json:"concrete"`
	} `json:"locations"`
}

// runZizmor implements the github-actions-specific half of "scan"
// (the spec's own literal line: "checkov, trivy config, zizmor,
// kyverno apply"). zizmor scans a directory itself (it recursively
// finds .github/workflows/*.yml under whatever path it is given, not
// a single file — confirmed for real), so this runner passes "."
// against the target's own resolved input directory exactly like
// runCheckov/runTrivyConfig already do.
func runZizmor(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	dir, err := targetInputDir(in)
	if err != nil {
		return nil, err
	}

	stdout, ok, err := runToolJSON(ctx, x, "zizmor", []string{"--format", "json", "."}, dir, timeout)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	var report []zizmorFinding
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		return nil, fmt.Errorf("check: zizmor: output does not parse: %w", err)
	}

	var findings []Finding
	for _, f := range report {
		sev := CapSeverity(strings.ToLower(f.Determinations.Severity), in.Check.Severity)
		file := ""
		line := 0
		if len(f.Locations) > 0 {
			file = f.Locations[0].Symbolic.Key.Local.VerbatimPath
			line = f.Locations[0].Concrete.Location.StartPoint.Row + 1 // zizmor's own row is 0-indexed
		}
		msg := fmt.Sprintf("[%s] %s", f.Ident, f.Desc)
		findings = append(findings, scanFinding(in, "zizmor", file, line, msg, sev))
	}
	return findings, nil
}
