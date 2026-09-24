package jenkins_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
)

func TestRenderPreservesOriginalBaseName(t *testing.T) {
	cases := []struct {
		fixture, name string
	}{
		{"testdata/Jenkinsfile", "Jenkinsfile"},
		{"testdata/jobs/build.xml", "build.xml"},
		{"testdata/jobs/build.groovy", "build.groovy"},
	}
	for _, c := range cases {
		src := filepath.Join(t.TempDir(), c.name)
		mustCopyFile(t, c.fixture, src)
		out := t.TempDir()

		a := jenkins.New(nil, devConfig())
		r, err := a.Render(context.Background(), ops.Target{Path: src}, ops.RefBranch, out)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r.Marked != nil {
			t.Errorf("%s: Marked = %v, want nil", c.name, r.Marked)
		}
		if _, err := os.Stat(filepath.Join(out, c.name)); err != nil {
			t.Errorf("%s: rendering did not preserve the original base name: %v", c.name, err)
		}
	}
}
