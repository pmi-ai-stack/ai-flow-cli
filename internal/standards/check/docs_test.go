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

func TestRunDocsNoTicketDirNoFindings(t *testing.T) {
	runner, ok := check.ForKind("docs")
	if !ok {
		t.Fatal("no docs runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "docs.adr"},
		Repo:  check.Repo{Name: "acme.api", LocalPath: t.TempDir()},
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none for the ticket-free form", findings)
	}
}

func TestRunDocsArchitecturalRequiresADR(t *testing.T) {
	ticketDir := t.TempDir() + "/PROJ-1"
	writeFile(t, filepath.Join(ticketDir, "plan.md"), "# Plan\n\nArchitectural: yes\n")
	repoDir := t.TempDir()

	runner, _ := check.ForKind("docs")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "docs.adr"},
		Repo:      check.Repo{Name: "acme.api", LocalPath: repoDir},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1 (no ADR present)", findings)
	}

	writeFile(t, filepath.Join(repoDir, "docs", "adr", "0001-PROJ-1-do-the-thing.md"), "# ADR\n")
	findings, err = runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "docs.adr"},
		Repo:      check.Repo{Name: "acme.api", LocalPath: repoDir},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none once the ADR exists", findings)
	}
}

func TestRunDocsChangelogEntry(t *testing.T) {
	ticketDir := t.TempDir() + "/PROJ-2"
	writeFile(t, filepath.Join(ticketDir, "plan.md"), "# Plan\n")
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "CHANGELOG.md"), "## Unreleased\n\n- fixed something else\n")

	runner, _ := check.ForKind("docs")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "docs.changelog"},
		Repo:      check.Repo{Name: "acme.api", LocalPath: repoDir},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "CHANGELOG.md" {
		t.Fatalf("findings = %#v", findings)
	}

	writeFile(t, filepath.Join(repoDir, "CHANGELOG.md"), "## Unreleased\n\n- PROJ-2: fixed something\n")
	findings, err = runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "docs.changelog"},
		Repo:      check.Repo{Name: "acme.api", LocalPath: repoDir},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none once the CHANGELOG mentions the ticket key", findings)
	}
}

func TestRunDocsNoChangelogFileIsFine(t *testing.T) {
	ticketDir := t.TempDir() + "/PROJ-3"
	writeFile(t, filepath.Join(ticketDir, "plan.md"), "# Plan\n")
	repoDir := t.TempDir()

	runner, _ := check.ForKind("docs")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check:     standards.EffectiveCheck{ID: "docs.changelog"},
		Repo:      check.Repo{Name: "acme.api", LocalPath: repoDir},
		TicketDir: ticketDir,
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none: no CHANGELOG.md means the check does not apply", findings)
	}
}
