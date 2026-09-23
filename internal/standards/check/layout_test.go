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

const layoutConfigJSON = `{
  "required_files": ["go.mod"],
  "test_placement": "sibling",
  "test_suffix": ["_test.go"],
  "forbidden_paths": ["vendor/**"]
}`

func TestRunLayoutMissingRequiredFile(t *testing.T) {
	dir := t.TempDir()
	c := standards.EffectiveCheck{ID: "go.layout", Config: "layout.json", Pack: packWithConfig("layout.json", layoutConfigJSON)}
	runner, ok := check.ForKind("layout")
	if !ok {
		t.Fatal("no layout runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Repo: check.Repo{Name: "acme.api", LocalPath: dir},
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "go.mod" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestRunLayoutForbiddenPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/acme\n")
	writeFile(t, filepath.Join(dir, "vendor", "github.com", "x", "x.go"), "package x\n")
	c := standards.EffectiveCheck{ID: "go.layout", Config: "layout.json", Pack: packWithConfig("layout.json", layoutConfigJSON)}
	runner, _ := check.ForKind("layout")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Repo: check.Repo{Name: "acme.api", LocalPath: dir},
	}, exectest.New(t), time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "vendor" {
		t.Fatalf("findings = %#v, want one finding on the vendor/ directory itself", findings)
	}
}

func TestRunLayoutClean(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/acme\n")
	c := standards.EffectiveCheck{ID: "go.layout", Config: "layout.json", Pack: packWithConfig("layout.json", layoutConfigJSON)}
	runner, _ := check.ForKind("layout")
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
