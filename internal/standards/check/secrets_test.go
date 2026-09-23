package check_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

func TestRunSecretsGitleaksFindsLeak(t *testing.T) {
	configPath := filepath.Join(os.TempDir(), "ai-flow-check-config-org.secrets.toml")
	reportPath := filepath.Join(os.TempDir(), "ai-flow-check-report-org.secrets.json")
	defer os.Remove(configPath) // best-effort cleanup
	defer os.Remove(reportPath) // best-effort cleanup

	if err := os.WriteFile(reportPath, []byte(`[{"File":"config.yaml","StartLine":4,"Description":"AWS key","RuleID":"aws-access-key"}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	x := exectest.New(t).On("gitleaks", []string{
		"detect", "--source", ".", "--no-git", "--report-format", "json",
		"--report-path", reportPath, "--exit-code", "0", "--config", configPath,
	}, exec.Output{})

	c := standards.EffectiveCheck{
		ID: "org.secrets", Tool: "gitleaks", Severity: "must", Config: "gitleaks.toml",
		Pack: packWithConfig("gitleaks.toml", "title = \"test\"\n"),
	}
	runner, ok := check.ForKind("secrets")
	if !ok {
		t.Fatal("no secrets runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Repo: check.Repo{Name: "acme.api", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1", findings)
	}
	f := findings[0]
	if f.File != "config.yaml" || f.Line != 4 || f.Status != "open" {
		t.Errorf("finding = %+v", f)
	}
	if f.Message != "[aws-access-key] AWS key" {
		t.Errorf("message = %q", f.Message)
	}
}

func TestRunSecretsGitleaksCleanNoReportFile(t *testing.T) {
	configPath := filepath.Join(os.TempDir(), "ai-flow-check-config-org.secrets2.toml")
	reportPath := filepath.Join(os.TempDir(), "ai-flow-check-report-org.secrets2.json")
	defer os.Remove(configPath) // best-effort cleanup
	_ = os.Remove(reportPath)   // gitleaks writes no report file at all when it finds nothing

	x := exectest.New(t).On("gitleaks", []string{
		"detect", "--source", ".", "--no-git", "--report-format", "json",
		"--report-path", reportPath, "--exit-code", "0", "--config", configPath,
	}, exec.Output{})

	c := standards.EffectiveCheck{
		ID: "org.secrets2", Tool: "gitleaks", Severity: "must", Config: "gitleaks.toml",
		Pack: packWithConfig("gitleaks.toml", "title = \"test\"\n"),
	}
	runner, _ := check.ForKind("secrets")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Repo: check.Repo{Name: "acme.api", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}
