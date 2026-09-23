package terraform_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestRenderCopiesInitsAndValidates(t *testing.T) {
	srcDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(srcDir, "main.tf"))
	outDir := t.TempDir()
	home := t.TempDir()

	x := exectest.New(t)
	x.On("terraform", []string{"init", "-backend=false", "-input=false", "-no-color"}, exec.Output{})
	x.On("terraform", []string{"validate", "-no-color"}, exec.Output{})

	a := terraform.New(x, &config.Config{}, home)
	r, err := a.Render(context.Background(), ops.Target{Addon: "terraform", Path: srcDir}, ops.RefBranch, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Marked != nil {
		t.Errorf("Marked = %v, want nil", r.Marked)
	}
	if _, err := os.Stat(filepath.Join(outDir, "main.tf")); err != nil {
		t.Errorf("main.tf not copied into out: %v", err)
	}
	for _, c := range x.Calls {
		if c.Dir != outDir {
			t.Errorf("call %v Dir = %q, want %q", c.Args, c.Dir, outDir)
		}
	}
}

func TestRenderSkipsDotTerraformOnCopy(t *testing.T) {
	srcDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(srcDir, "main.tf"))
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(srcDir, ".terraform", "modules", "x", "main.tf"))
	outDir := t.TempDir()

	x := exectest.New(t)
	x.On("terraform", []string{"init", "-backend=false", "-input=false", "-no-color"}, exec.Output{})
	x.On("terraform", []string{"validate", "-no-color"}, exec.Output{})

	a := terraform.New(x, &config.Config{}, t.TempDir())
	if _, err := a.Render(context.Background(), ops.Target{Path: srcDir}, ops.RefBranch, outDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".terraform")); err == nil {
		t.Error(".terraform/ was copied into out, want it skipped")
	}
}

func TestRenderPropagatesInitFailure(t *testing.T) {
	srcDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(srcDir, "main.tf"))
	outDir := t.TempDir()

	x := exectest.New(t)
	x.OnFail("terraform", []string{"init", "-backend=false", "-input=false", "-no-color"}, &envelope.Error{Code: "tool_failed", Message: "boom"})

	a := terraform.New(x, &config.Config{}, t.TempDir())
	if _, err := a.Render(context.Background(), ops.Target{Path: srcDir}, ops.RefBranch, outDir); err == nil {
		t.Fatal("Render() err = nil, want the init failure propagated")
	}
}
