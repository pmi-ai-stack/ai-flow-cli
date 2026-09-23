package check_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

func TestRunPRRequiredHeadings(t *testing.T) {
	ticketDir := t.TempDir() + "/PROJ-11"
	writeFile(t, filepath.Join(ticketDir, "pr-body.md"), "## Summary\n\nDid a thing.\n")

	runner, ok := check.ForKind("pr")
	if !ok {
		t.Fatal("no pr runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "process.pr"},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1 (missing Test plan)", findings)
	}
}

func TestRunPRClean(t *testing.T) {
	ticketDir := t.TempDir() + "/PROJ-12"
	writeFile(t, filepath.Join(ticketDir, "pr-body.md"), "## Summary\n\nDid a thing.\n\n## Test plan\n\n- [x] ran the tests\n")

	runner, _ := check.ForKind("pr")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "process.pr"},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestRunPRTooLargeWithoutJustification(t *testing.T) {
	ticketDir := t.TempDir() + "/PROJ-13"
	body := "## Summary\n\n" + strings.Repeat("x", 21000) + "\n\n## Test plan\n\n- done\n"
	writeFile(t, filepath.Join(ticketDir, "pr-body.md"), body)

	runner, _ := check.ForKind("pr")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "process.pr"},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1 (too large, no justification)", findings)
	}
}
