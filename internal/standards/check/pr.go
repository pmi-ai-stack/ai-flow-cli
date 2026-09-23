package check

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// requiredPRHeadings are pr-body.md's own required sections — the same
// two this codebase's own "Creating pull requests" convention already
// uses (AGENTS.md), reused here as the check's own default rather than
// inventing a different pair.
var requiredPRHeadings = []string{"## Summary", "## Test plan"}

const prBodyMaxLen = 20000

// runPR implements the "pr" kind: a built-in check against
// pr-body.md, applicable only at --stage ship. Tolerant of a missing
// file, the same way runCommit is.
func runPR(_ context.Context, in RunInput, _ exec.Execer, _ time.Duration) ([]Finding, error) {
	if in.TicketDir == "" {
		return nil, nil
	}
	data, ok, err := readFileIfExists(filepath.Join(in.TicketDir, "pr-body.md"))
	if err != nil || !ok {
		return nil, err
	}
	body := string(data)

	var findings []Finding
	for _, h := range requiredPRHeadings {
		if !strings.Contains(body, h) {
			findings = append(findings, prFinding(fmt.Sprintf("pr-body.md is missing a %q section", h)))
		}
	}
	if len(body) > prBodyMaxLen && !strings.Contains(body, "## Why so large") {
		findings = append(findings, prFinding(
			fmt.Sprintf("pr-body.md is %d characters, over %d, with no \"## Why so large\" justification", len(body), prBodyMaxLen)))
	}
	return findings, nil
}

func prFinding(message string) Finding {
	return Finding{File: "pr-body.md", Message: message, Status: "open"}
}
