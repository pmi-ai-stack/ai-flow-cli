package githubactions

import (
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// Harnesses lists, for every job t's own workflow file defines, one
// Kind: HarnessUnit entry (the matrix's own "act -j <job> -n" cell)
// and one Kind: HarnessE2E entry (the matrix's own "act -j <job>"
// opt-in cell) — detection and naming only (the phase-27 plan's own
// §0.8/§1.2.1): internal/ops/harness.ToolFor recognizes neither name
// yet, silently dropped by `ops test run` exactly as
// internal/cli/ops_infratest.go's own doc comment already documents,
// the identical posture phase 26 already established for helm's own
// three unwired harness tool names. Reads t's own file from repo's
// checkout directly (Harnesses receives detect.Repo/Target, never a
// *Rendered) — never a subprocess.
func (a *Addon) Harnesses(repo detect.Repo, t ops.Target) ([]ops.Harness, error) {
	jobs, err := workflowJobs(filepath.Join(repo.LocalPath, filepath.FromSlash(t.Path)))
	if err != nil {
		return nil, err
	}
	ids := sortedJobIDs(jobs)

	harnesses := make([]ops.Harness, 0, len(ids)*2)
	for _, id := range ids {
		addr := t.Path + "#" + id
		harnesses = append(harnesses, ops.Harness{Name: addr + "#act-n", Kind: ops.HarnessUnit})
		harnesses = append(harnesses, ops.Harness{Name: addr + "#act-run", Kind: ops.HarnessE2E})
	}
	return harnesses, nil
}
