package check

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// architecturalMarkerRE is plan.md's own opt-in signal that a change
// is architectural and needs an ADR — a plain line, not a dependency
// on plan.md's full heading grammar (phase 15's own mandate).
var architecturalMarkerRE = regexp.MustCompile(`(?im)^architectural:\s*yes\s*$`)

// runDocs implements the "docs" kind: ADR-present and CHANGELOG-entry
// checks, both ticket-only (the phase-14 plan's design decision 4) —
// with no TicketDir (the `standards check <dir>` form) there is no
// plan.md and no ticket key to look for, so this returns no findings
// at all rather than inventing an answer. "Public API comments" (the
// spec's third docs example) is not implemented — see the plan's
// design decision 13.
func runDocs(_ context.Context, in RunInput, _ exec.Execer, _ time.Duration) ([]Finding, error) {
	if in.TicketDir == "" {
		return nil, nil
	}

	var findings []Finding

	if f, err := checkADR(in); err != nil {
		return nil, err
	} else if f != nil {
		findings = append(findings, *f)
	}

	if f, err := checkChangelog(in); err != nil {
		return nil, err
	} else if f != nil {
		findings = append(findings, *f)
	}

	return findings, nil
}

func checkADR(in RunInput) (*Finding, error) {
	planPath := filepath.Join(in.TicketDir, "plan.md")
	data, ok, err := readFileIfExists(planPath)
	if err != nil || !ok || !architecturalMarkerRE.Match(data) {
		return nil, err
	}

	key := ticketKeyFromDir(in.TicketDir)
	for _, dir := range []string{"docs/adr", "adr"} {
		matches, err := filepath.Glob(filepath.Join(in.Repo.LocalPath, dir, "*"+key+"*"))
		if err != nil {
			return nil, fmt.Errorf("check: docs: glob %s: %w", dir, err)
		}
		if len(matches) > 0 {
			return nil, nil
		}
	}

	return &Finding{
		Repo: in.Repo.Name,
		Message: fmt.Sprintf(
			"plan.md marks this change architectural; no ADR under docs/adr/ or adr/ names %s", key,
		),
		Status: "open",
	}, nil
}

func checkChangelog(in RunInput) (*Finding, error) {
	path := filepath.Join(in.Repo.LocalPath, "CHANGELOG.md")
	data, ok, err := readFileIfExists(path)
	if err != nil || !ok {
		return nil, err
	}

	key := ticketKeyFromDir(in.TicketDir)
	if strings.Contains(string(data), key) {
		return nil, nil
	}
	return &Finding{
		Repo:    in.Repo.Name,
		File:    "CHANGELOG.md",
		Message: fmt.Sprintf("CHANGELOG.md has no entry referencing %s", key),
		Status:  "open",
	}, nil
}

// ticketKeyFromDir recovers the ticket key from its handoff directory
// name (~/.ai-flow/<KEY>/) — the one piece of ticket identity docs.go
// needs that RunInput does not otherwise carry.
func ticketKeyFromDir(ticketDir string) string {
	return filepath.Base(filepath.Clean(ticketDir))
}
