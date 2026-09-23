package check

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// runFormat implements the "format" kind: gofmt (go), prettier
// (typescript/javascript), ruff format (python). --fix is the one kind
// this phase auto-applies (the spec's own "fix: auto" marker on the
// format row, and only that row — see the phase-14 plan's design
// decision 6); csharp/terraform formatters named in the spec's own
// table as further examples are not implemented (no addon/language
// support exists yet for either — decision 12/15's language scope).
func runFormat(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	switch in.Check.Tool {
	case "gofmt":
		return runGofmt(ctx, in, x, timeout)
	case "prettier":
		return runPrettier(ctx, in, x, timeout)
	case "ruff":
		return runRuffFormat(ctx, in, x, timeout)
	default:
		return nil, fmt.Errorf("check: format: no runner for tool %q", in.Check.Tool)
	}
}

func formatFinding(in RunInput, file string) Finding {
	return Finding{
		Tool: in.Check.Tool, Repo: in.Repo.Name, File: file,
		Message: fmt.Sprintf("%s: not formatted", file),
		Status:  "open",
	}
}

// runGofmt: `gofmt -l .` lists non-conforming files (one per line);
// `gofmt -w .` (Fix) rewrites them in place. gofmt recurses into a
// directory argument on its own, skipping dot-directories.
func runGofmt(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	out, err := x.Run(ctx, "gofmt", []string{"-l", "."}, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout})
	if err != nil {
		return nil, err
	}
	files := nonEmptyLines(out.Stdout)
	return applyFixOrReport(ctx, in, x, timeout, "gofmt", []string{"-w", "."}, files)
}

// runPrettier: `prettier --check .` prints one non-conforming path
// per line (optionally "[warn] "-prefixed, depending on version); a
// summary line ("Checking formatting...", "Code style issues found...")
// is filtered out by requiring the line to look like a path (no
// spaces once the optional prefix is stripped). Invoked directly, not
// through `npx`, on the same "the tool is on PATH" assumption every
// other check runner in this package makes (gofmt, golangci-lint,
// ruff, checkstyle, gitleaks) — a project whose prettier only exists as
// a local devDependency needs it on PATH some other way (a
// project-local shim, a global install), which is this phase's own
// scope line, not a gap it owes silently.
func runPrettier(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	out, err := x.Run(ctx, "prettier", []string{"--check", "."}, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return nil, err
	}
	var files []string
	for _, line := range nonEmptyLines(out.Stdout + "\n" + out.Stderr) {
		line = strings.TrimPrefix(line, "[warn] ")
		if strings.Contains(line, " ") || strings.HasPrefix(line, "Checking") || strings.HasPrefix(line, "Code style") {
			continue
		}
		files = append(files, line)
	}

	return applyFixOrReport(ctx, in, x, timeout, "prettier", []string{"--write", "."}, files)
}

// runRuffFormat: `ruff format --check --config <path> .` prints
// "Would reformat: <path>" per file; the fix form drops --check.
func runRuffFormat(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	configPath, cleanup, err := materializeConfig(in.Check)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"format", "--check", "."}
	if configPath != "" {
		args = []string{"format", "--check", "--config", configPath, "."}
	}
	out, err := x.Run(ctx, "ruff", args, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return nil, err
	}
	var files []string
	for _, line := range nonEmptyLines(out.Stdout) {
		const prefix = "Would reformat: "
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		files = append(files, strings.TrimPrefix(line, prefix))
	}

	fixArgs := []string{"format", "."}
	if configPath != "" {
		fixArgs = []string{"format", "--config", configPath, "."}
	}
	return applyFixOrReport(ctx, in, x, timeout, "ruff", fixArgs, files)
}

// applyFixOrReport is runGofmt/runPrettier/runRuffFormat's shared
// check-then-fix tail: with Fix requested and something to fix, run
// the tool's own write-mode invocation and report every named file as
// fixed; otherwise report each as an open finding. fixArgs is the argv
// the tool's write mode needs (e.g. gofmt's "-w .", ruff format's
// "format .").
func applyFixOrReport(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration, tool string, fixArgs, files []string) ([]Finding, error) {
	if in.Fix {
		if len(files) == 0 {
			return nil, nil
		}
		if _, err := x.Run(ctx, tool, fixArgs, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout}); err != nil {
			return nil, err
		}
		return fixedFindings(in, files), nil
	}
	findings := make([]Finding, 0, len(files))
	for _, f := range files {
		findings = append(findings, formatFinding(in, f))
	}
	return findings, nil
}

func fixedFindings(in RunInput, files []string) []Finding {
	findings := make([]Finding, 0, len(files))
	for _, f := range files {
		findings = append(findings, Finding{
			Tool: in.Check.Tool, Repo: in.Repo.Name, File: f,
			Message: fmt.Sprintf("%s: reformatted", f),
			Status:  "fixed", AutofixApplied: true,
		})
	}
	return findings
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
