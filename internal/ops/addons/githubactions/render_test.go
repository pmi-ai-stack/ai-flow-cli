package githubactions_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
)

func TestRenderCopiesFileToFixedName(t *testing.T) {
	src := filepath.Join(t.TempDir(), "ci.yml")
	mustCopyFile(t, "testdata/ci.yml", src)
	out := t.TempDir()

	a := githubactions.New(nil, devConfig())
	r, err := a.Render(context.Background(), ops.Target{Path: src}, ops.RefBranch, out)
	if err != nil {
		t.Fatal(err)
	}
	if r.Marked != nil {
		t.Errorf("Marked = %v, want nil", r.Marked)
	}
	got, err := os.ReadFile(filepath.Join(out, githubactions.RenderedFile))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile("testdata/ci.yml")
	if string(got) != string(want) {
		t.Errorf("rendered content does not match the source file byte for byte")
	}
}
