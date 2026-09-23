package check_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

func TestRunCommitConventionalAndKey(t *testing.T) {
	ticketDir := t.TempDir() + "/PROJ-9"
	writeFile(t, filepath.Join(ticketDir, "commit-message.md"), "feat(api): add the widget endpoint (PROJ-9)\n\nBody text.\n")

	runner, ok := check.ForKind("commit")
	if !ok {
		t.Fatal("no commit runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "process.commit-conventional"},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestRunCommitViolations(t *testing.T) {
	ticketDir := t.TempDir() + "/PROJ-10"
	writeFile(t, filepath.Join(ticketDir, "commit-message.md"),
		"did a bunch of stuff that is way too long to fit under the seventy two character limit for a subject line\n")

	runner, _ := check.ForKind("commit")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "process.commit-conventional"},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 3 {
		t.Fatalf("findings = %#v, want 3 (not conventional, too long, no ticket key)", findings)
	}
}

func TestRunCommitNoFileNoFindings(t *testing.T) {
	runner, _ := check.ForKind("commit")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "process.commit-conventional"},
		TicketDir: t.TempDir(),
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none before the ship skill has composed commit-message.md", findings)
	}
}
