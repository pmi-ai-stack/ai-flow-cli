package check

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// runLint implements the "lint" kind: golangci-lint (go), eslint
// (typescript/javascript), ruff check (python), checkstyle (java).
// Lint never autofixes through this package — in.Fix is ignored here
// even when Tool is "ruff" (the same binary format.go's "format" kind
// uses): the spec's fix: auto marker is on the format row only (the
// phase-14 plan's design decision 6), and lint findings are fixed by
// the skill editing code, at the source, in the spirit of the
// guidance — not by a linter's own --fix flag rewriting it first.
//
// Output-format flags below are this phase's own documented choice,
// not verified against a real binary in this development environment
// (none of golangci-lint/eslint/ruff/checkstyle is installed here) —
// flagged provisional the same way phase 9 flagged Cursor's payload
// field names and phase 12 flagged the Jenkins linter's binary name,
// pending confirmation against a real tool run (`-tags checktools`).
func runLint(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	switch in.Check.Tool {
	case "golangci-lint":
		return runGolangciLint(ctx, in, x, timeout)
	case "eslint":
		return runESLint(ctx, in, x, timeout)
	case "ruff":
		return runRuffCheck(ctx, in, x, timeout)
	case "checkstyle":
		return runCheckstyle(ctx, in, x, timeout)
	default:
		return nil, fmt.Errorf("check: lint: no runner for tool %q", in.Check.Tool)
	}
}

func lintFinding(in RunInput, file string, line int, message string) Finding {
	return Finding{
		Tool: in.Check.Tool, Repo: in.Repo.Name, File: file, Line: line,
		Message: message, Status: "open",
	}
}

// golangciLintReport is golangci-lint v2's JSON report shape
// (`--output.json.path stdout`), trimmed to what this runner reads.
type golangciLintReport struct {
	Issues []struct {
		Text string `json:"Text"`
		Pos  struct {
			Filename string `json:"Filename"`
			Line     int    `json:"Line"`
		} `json:"Pos"`
		FromLinter string `json:"FromLinter"`
	} `json:"Issues"`
}

func runGolangciLint(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	configPath, cleanup, err := materializeConfig(in.Check)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// golangci-lint v2 — the version the reference pack's own
	// `version: "2"` config requires — removed v1's `--out-format json`
	// (v2 exits 3 "unknown flag", which the old empty-stdout tolerance
	// read as a clean pass, so go.lint had never reported anything).
	// v2 also resolves report paths relative to the config file by
	// default, and the config is materialized in a temp directory, so
	// paths are requested absolute and made repository-relative below.
	args := []string{"run", "--output.json.path", "stdout", "--show-stats=false", "--path-mode", "abs"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, "./...")
	out, err := x.Run(ctx, "golangci-lint", args, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout})
	if err := failedWithoutReport(out, err); err != nil {
		return nil, err
	}
	if out.Stdout == "" {
		return nil, nil
	}
	var report golangciLintReport
	if err := json.Unmarshal([]byte(out.Stdout), &report); err != nil {
		return nil, fmt.Errorf("check: golangci-lint: output does not parse: %w", err)
	}
	findings := make([]Finding, 0, len(report.Issues))
	for _, iss := range report.Issues {
		msg := iss.Text
		if iss.FromLinter != "" {
			msg = fmt.Sprintf("[%s] %s", iss.FromLinter, iss.Text)
		}
		findings = append(findings, lintFinding(in, repoRelative(in.Repo.LocalPath, iss.Pos.Filename), iss.Pos.Line, msg))
	}
	return findings, nil
}

// repoRelative is file relative to repoDir when it lies inside it, else
// file unchanged — findings name repository-relative paths.
func repoRelative(repoDir, file string) string {
	if !filepath.IsAbs(file) {
		return file
	}
	root, err := filepath.Abs(repoDir)
	if err != nil {
		return file
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return file
	}
	return filepath.ToSlash(rel)
}

// esLintReport is eslint's `--format json` shape, trimmed.
type esLintReport []struct {
	FilePath string `json:"filePath"`
	Messages []struct {
		Line    int    `json:"line"`
		Message string `json:"message"`
		RuleID  string `json:"ruleId"`
	} `json:"messages"`
}

func runESLint(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	configPath, cleanup, err := materializeConfig(in.Check)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"--format", "json"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, ".")
	out, err := x.Run(ctx, "eslint", args, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout})
	if err := failedWithoutReport(out, err); err != nil {
		return nil, err
	}
	if out.Stdout == "" {
		return nil, nil
	}
	var report esLintReport
	if err := json.Unmarshal([]byte(out.Stdout), &report); err != nil {
		return nil, fmt.Errorf("check: eslint: output does not parse: %w", err)
	}
	var findings []Finding
	for _, f := range report {
		for _, m := range f.Messages {
			msg := m.Message
			if m.RuleID != "" {
				msg = fmt.Sprintf("[%s] %s", m.RuleID, m.Message)
			}
			findings = append(findings, lintFinding(in, f.FilePath, m.Line, msg))
		}
	}
	return findings, nil
}

// ruffCheckReport is `ruff check --output-format json`'s shape,
// trimmed.
type ruffCheckReport []struct {
	Filename string `json:"filename"`
	Message  string `json:"message"`
	Code     string `json:"code"`
	Location struct {
		Row int `json:"row"`
	} `json:"location"`
}

func runRuffCheck(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	configPath, cleanup, err := materializeConfig(in.Check)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"check", "--output-format", "json"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, ".")
	out, err := x.Run(ctx, "ruff", args, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout})
	if err := failedWithoutReport(out, err); err != nil {
		return nil, err
	}
	if out.Stdout == "" {
		return nil, nil
	}
	var report ruffCheckReport
	if err := json.Unmarshal([]byte(out.Stdout), &report); err != nil {
		return nil, fmt.Errorf("check: ruff: output does not parse: %w", err)
	}
	findings := make([]Finding, 0, len(report))
	for _, r := range report {
		msg := r.Message
		if r.Code != "" {
			msg = fmt.Sprintf("[%s] %s", r.Code, r.Message)
		}
		findings = append(findings, lintFinding(in, r.Filename, r.Location.Row, msg))
	}
	return findings, nil
}

// checkstyleReport is checkstyle's own `-f xml` output shape, trimmed.
type checkstyleReport struct {
	XMLName xml.Name `xml:"checkstyle"`
	Files   []struct {
		Name   string `xml:"name,attr"`
		Errors []struct {
			Line     int    `xml:"line,attr"`
			Message  string `xml:"message,attr"`
			Severity string `xml:"severity,attr"`
		} `xml:"error"`
	} `xml:"file"`
}

func runCheckstyle(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	configPath, cleanup, err := materializeConfig(in.Check)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"-f", "xml"}
	if configPath != "" {
		args = append(args, "-c", configPath)
	}
	args = append(args, ".")
	out, err := x.Run(ctx, "checkstyle", args, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout})
	if err := failedWithoutReport(out, err); err != nil {
		return nil, err
	}
	if out.Stdout == "" {
		return nil, nil
	}
	var report checkstyleReport
	if err := xml.Unmarshal([]byte(out.Stdout), &report); err != nil {
		return nil, fmt.Errorf("check: checkstyle: output does not parse: %w", err)
	}
	var findings []Finding
	for _, f := range report.Files {
		for _, e := range f.Errors {
			findings = append(findings, lintFinding(in, f.Name, e.Line, e.Message))
		}
	}
	return findings, nil
}
