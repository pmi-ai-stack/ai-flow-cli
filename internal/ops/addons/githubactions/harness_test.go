package githubactions_test

import (
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
)

func TestHarnessesDetectPerJobUnitAndE2E(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/ci-two-jobs.yml", filepath.Join(repoDir, ".github", "workflows", "ci.yml"))

	a := githubactions.New(nil, devConfig())
	repo := detect.Repo{Name: "acme.svc", LocalPath: repoDir}
	target := ops.Target{Addon: "github-actions", Path: ".github/workflows/ci.yml"}

	harnesses, err := a.Harnesses(repo, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(harnesses) != 4 {
		t.Fatalf("harnesses = %v, want 4 (unit+e2e per job, 2 jobs)", harnesses)
	}
	var unit, e2e int
	for _, h := range harnesses {
		switch h.Kind {
		case ops.HarnessUnit:
			unit++
		case ops.HarnessE2E:
			e2e++
		}
	}
	if unit != 2 || e2e != 2 {
		t.Errorf("unit=%d e2e=%d, want 2 and 2", unit, e2e)
	}
}
