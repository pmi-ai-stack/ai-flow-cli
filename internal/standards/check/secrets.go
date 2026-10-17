package check

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// gitleaksFinding is one entry of gitleaks' own `--report-format json`
// array, trimmed to what this runner reads.
type gitleaksFinding struct {
	File        string `json:"File"`
	StartLine   int    `json:"StartLine"`
	Description string `json:"Description"`
	RuleID      string `json:"RuleID"`
}

// runSecrets implements the "secrets" kind: `gitleaks detect` against
// the repository's own checkout. --exit-code 0 is passed deliberately
// so a leak never turns into exec.IsToolFailed's tool_failed path —
// this runner reads the report file itself and decides what a leak
// means (a Finding, at the check's own always-must severity — secrets
// is standards.UnwaivableKind), not gitleaks' own process exit code.
//
// Scoped to the whole repository, not literally "the diff" as the
// spec's own table entry says: internal/standards/check has no diff
// primitive of its own (that lives in internal/gitops, a layer above
// the resolver this package serves), and --no-git treats the checkout
// as a plain directory rather than walking its commit history, which
// is a reasonable, cheaper superset of "the diff" for a first pass —
// the reference pack's own gitleaks.toml already carries a testdata/
// allowlist anticipating exactly this from phase 12.
func runSecrets(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	// The report path is checked first so a missing scratch directory
	// fails before anything is written.
	reportPath, err := scratchPath(in, "report-"+in.Check.ID+".json")
	if err != nil {
		return nil, err
	}
	configPath, cleanup, err := materializeConfig(in)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Deterministic within the private scratch directory, for the same
	// reason materializeConfig's own path is: a caller's test scripts
	// the exact argv it expects (see runner.go's doc comment).
	defer func() { _ = os.Remove(reportPath) }()

	args := []string{"detect", "--source", ".", "--no-git", "--report-format", "json", "--report-path", reportPath, "--exit-code", "0"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	if _, err := x.Run(ctx, "gitleaks", args, exec.RunOpts{Dir: in.Repo.LocalPath, Timeout: timeout}); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(reportPath) //nolint:gosec // G304: reportPath is this function's own temp file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no leaks: gitleaks writes no report file at all
		}
		return nil, fmt.Errorf("check: secrets: read gitleaks report: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	var leaks []gitleaksFinding
	if err := json.Unmarshal(data, &leaks); err != nil {
		return nil, fmt.Errorf("check: gitleaks: output does not parse: %w", err)
	}

	findings := make([]Finding, 0, len(leaks))
	for _, l := range leaks {
		msg := l.Description
		if l.RuleID != "" {
			msg = fmt.Sprintf("[%s] %s", l.RuleID, l.Description)
		}
		findings = append(findings, Finding{
			Tool: in.Check.Tool, Repo: in.Repo.Name, File: l.File, Line: l.StartLine,
			Message: msg, Status: "open",
		})
	}
	return findings, nil
}
