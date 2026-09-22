package render

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff/handofftest"
)

// writeRepo writes a minimal, schema-valid repo.json to dir.
func writeRepo(t *testing.T, dir string) {
	t.Helper()
	artifact := map[string]any{
		"team": "Team Beta",
		"repositories": []map[string]any{
			{"name": "acme.billing.api", "local_path": "/work/acme.billing.api", "instructions_file": "AGENTS.md"},
			{"name": "acme.billing.web", "local_path": "/work/acme.billing.web", "instructions_file": "CLAUDE.md"},
		},
	}
	if _, err := handoff.WriteArtifact(dir, "repo.json", "repo", artifact); err != nil {
		t.Fatalf("write repo.json: %v", err)
	}
}

// writeMutation writes a minimal, schema-valid mutation-results.json to
// dir. accepted is nil for "no accepted_below_threshold recorded".
func writeMutation(t *testing.T, dir string, accepted map[string]any) {
	t.Helper()
	artifact := map[string]any{
		"round": 1,
		"repositories": []map[string]any{
			{"repo": "acme.billing.api", "tool": "gremlins", "score": 85.5, "threshold": 80, "survivors": 3, "killed": 17},
		},
		"accepted_below_threshold": accepted,
	}
	if _, err := handoff.WriteArtifact(dir, "mutation-results.json", "mutation-results", artifact); err != nil {
		t.Fatalf("write mutation-results.json: %v", err)
	}
}

func TestLoad_MissingRepoJSON(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, "TEST-1")
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "missing_artifact" {
		t.Fatalf("Load: err = %v, want missing_artifact", err)
	}
}

func TestLoad_RepoOnly(t *testing.T) {
	dir := t.TempDir()
	writeRepo(t, dir)

	d, err := Load(dir, "TEST-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Key != "TEST-1" {
		t.Errorf("Key = %q, want TEST-1", d.Key)
	}
	if len(d.Repositories) != 2 {
		t.Fatalf("Repositories = %d entries, want 2", len(d.Repositories))
	}
	if d.Repositories[0].Name != "acme.billing.api" || d.Repositories[0].LocalPath != "/work/acme.billing.api" {
		t.Errorf("Repositories[0] = %+v", d.Repositories[0])
	}
	if d.Mutation != nil {
		t.Errorf("Mutation = %v, want nil (no mutation-results.json written)", d.Mutation)
	}
	if d.Accepted != nil {
		t.Errorf("Accepted = %+v, want nil", d.Accepted)
	}
}

func TestLoad_WithMutationAndAccepted(t *testing.T) {
	dir := t.TempDir()
	writeRepo(t, dir)
	writeMutation(t, dir, map[string]any{"by": "alice", "reason": "flaky external API, not worth chasing"})

	d, err := Load(dir, "TEST-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(d.Mutation) != 1 {
		t.Fatalf("Mutation = %d entries, want 1", len(d.Mutation))
	}
	m := d.Mutation[0]
	if m.Repo != "acme.billing.api" || m.Tool != "gremlins" || m.Score != 85.5 || m.Threshold != 80 || m.Survivors != 3 || m.Killed != 17 {
		t.Errorf("Mutation[0] = %+v", m)
	}
	if d.Accepted == nil || d.Accepted.By != "alice" || d.Accepted.Reason != "flaky external API, not worth chasing" {
		t.Errorf("Accepted = %+v", d.Accepted)
	}
}

func TestOutput_RepoOnly(t *testing.T) {
	d := Data{Key: "TEST-1", Repositories: []Repository{{Name: "acme.billing.api", LocalPath: "/work/acme.billing.api"}}}
	out := Output(d)

	for _, want := range []string{
		"# Summary\n",
		"**Plan:** [plan.md](plan.md)\n",
		"## Repositories\n",
		"- acme.billing.api (`/work/acme.billing.api`)\n",
		"## Checks, packs and waivers\n",
		"not implemented yet (see phases 12-14)",
		"## Mutation\n",
		"No mutation results recorded.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Output() missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestOutput_WithMutationAndAccepted(t *testing.T) {
	d := Data{
		Key:          "TEST-1",
		Repositories: []Repository{{Name: "acme.billing.api", LocalPath: "/work/acme.billing.api"}},
		Mutation:     []MutationRepo{{Repo: "acme.billing.api", Tool: "gremlins", Score: 85.5, Threshold: 80, Survivors: 3, Killed: 17}},
		Accepted:     &Accepted{By: "alice", Reason: "flaky external API"},
	}
	out := Output(d)

	for _, want := range []string{
		"| acme.billing.api | gremlins | 85.5 | 80 | 3 | 17 |",
		"Accepted below threshold by alice: flaky external API",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Output() missing %q\nfull output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "No mutation results recorded.") {
		t.Error("Output() still shows the no-mutation placeholder despite Mutation data")
	}
}

func TestOutput_NoRepositories(t *testing.T) {
	out := Output(Data{Key: "TEST-1"})
	if !strings.Contains(out, "(no repositories recorded)") {
		t.Errorf("Output() missing the no-repositories placeholder\nfull output:\n%s", out)
	}
}

func TestPRBlock_NoPlanLink(t *testing.T) {
	d := Data{Key: "TEST-1", Repositories: []Repository{{Name: "acme.billing.api"}}}
	out := PRBlock(d)

	if !strings.HasPrefix(out, "## ai-flow summary\n\n") {
		t.Errorf("PRBlock() does not start with the wrapping heading:\n%s", out)
	}
	if strings.Contains(out, "plan.md") {
		t.Error("PRBlock() links plan.md, which is never part of the git repository a hosted PR body is read from")
	}
	if !strings.Contains(out, "### Repositories\n") {
		t.Errorf("PRBlock() sections are not nested one level under its own heading:\n%s", out)
	}
	if strings.Contains(out, "# Summary\n") {
		t.Error("PRBlock() carries output.md's own top-level heading")
	}
}

func TestOutputAndPRBlock_ByteStable(t *testing.T) {
	d := Data{
		Key:          "TEST-1",
		Repositories: []Repository{{Name: "acme.billing.api", LocalPath: "/work/a"}, {Name: "acme.billing.web"}},
		Mutation:     []MutationRepo{{Repo: "acme.billing.api", Tool: "gremlins", Score: 85.5, Threshold: 80, Survivors: 3, Killed: 17}},
		Accepted:     &Accepted{By: "alice", Reason: "flaky"},
	}
	handofftest.RequireByteStable(t, func() ([]byte, error) { return []byte(Output(d)), nil })
	handofftest.RequireByteStable(t, func() ([]byte, error) { return []byte(PRBlock(d)), nil })
}

func TestLoad_MutationOptional_FilePathBuiltCorrectly(t *testing.T) {
	// Guards against a typo'd filename silently making Load always
	// treat mutation-results.json as absent.
	dir := t.TempDir()
	writeRepo(t, dir)
	writeMutation(t, dir, nil)

	if _, err := handoff.ReadArtifact(filepath.Join(dir, "mutation-results.json")); err != nil {
		t.Fatalf("sanity check: %v", err)
	}
	d, err := Load(dir, "TEST-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(d.Mutation) != 1 {
		t.Fatalf("Mutation = %d entries, want 1 (mutation-results.json was written)", len(d.Mutation))
	}
}
