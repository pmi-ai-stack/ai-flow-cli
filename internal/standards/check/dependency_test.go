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

const depConfig = `{
  "allowed_licenses": ["MIT"],
  "banned_packages": ["left-pad"],
  "minimum_versions": {"golang.org/x/mod": "0.15.0"},
  "fail_on_advisory_severity": "high"
}`

func TestRunDependencyGoModBannedAndMinimum(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/acme

go 1.22

require (
	golang.org/x/mod v0.10.0
	golang.org/x/text v0.14.0
)
`)
	c := standards.EffectiveCheck{
		ID: "go.dependencies", Config: "dependencies.json",
		Pack: packWithConfig("dependencies.json", depConfig),
	}
	runner, ok := check.ForKind("dependency")
	if !ok {
		t.Fatal("no dependency runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Repo: check.Repo{Name: "acme.api", LocalPath: dir},
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1 (golang.org/x/mod below minimum)", findings)
	}
	if findings[0].Target != "golang.org/x/mod" {
		t.Errorf("target = %q", findings[0].Target)
	}
}

func TestRunDependencyBannedPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"left-pad":"1.0.0","react":"^18.0.0"}}`)
	c := standards.EffectiveCheck{
		ID: "typescript.dependencies", Config: "dependencies.json",
		Pack: packWithConfig("dependencies.json", depConfig),
	}
	runner, _ := check.ForKind("dependency")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Repo: check.Repo{Name: "acme.web", LocalPath: dir},
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].Target != "left-pad" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestRunDependencyClean(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/acme\n\ngo 1.22\n")
	c := standards.EffectiveCheck{
		ID: "go.dependencies", Config: "dependencies.json",
		Pack: packWithConfig("dependencies.json", depConfig),
	}
	runner, _ := check.ForKind("dependency")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Repo: check.Repo{Name: "acme.api", LocalPath: dir},
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}
