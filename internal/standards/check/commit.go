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

// conventionalCommitRE matches a Conventional Commits subject line:
// type, optional (scope), colon, space, then the description.
var conventionalCommitRE = regexp.MustCompile(
	`^(feat|fix|refactor|perf|docs|style|test|build|ci|chore|revert)(\([\w.-]+\))?: .+`,
)

const commitSubjectMaxLen = 72

// runCommit implements the "commit" kind: a built-in check against
// commit-message.md, applicable only at --stage ship (the reference
// pack's own applies_to.stages). Tolerant of a missing file — there is
// none outside a ticket, and none yet inside one before the ship skill
// composes it (phase 15).
func runCommit(_ context.Context, in RunInput, _ exec.Execer, _ time.Duration) ([]Finding, error) {
	if in.TicketDir == "" {
		return nil, nil
	}
	data, ok, err := readFileIfExists(filepath.Join(in.TicketDir, "commit-message.md"))
	if err != nil || !ok {
		return nil, err
	}

	subject, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")

	var findings []Finding
	if !conventionalCommitRE.MatchString(subject) {
		findings = append(findings, commitFinding(
			"commit subject does not follow Conventional Commits (type(scope): subject)"))
	}
	if len(subject) > commitSubjectMaxLen {
		findings = append(findings, commitFinding(
			fmt.Sprintf("commit subject is %d characters, over the %d-character limit", len(subject), commitSubjectMaxLen)))
	}
	key := ticketKeyFromDir(in.TicketDir)
	if !strings.Contains(string(data), key) {
		findings = append(findings, commitFinding(fmt.Sprintf("commit message does not mention ticket key %s", key)))
	}
	return findings, nil
}

func commitFinding(message string) Finding {
	return Finding{File: "commit-message.md", Message: message, Status: "open"}
}
