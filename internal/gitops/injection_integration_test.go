//go:build integration

package gitops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// TestIntegrationBaseBranchCannotBecomeAGitOption is SEC-5a/SEC-6,
// reproduced with a real git before the fix (a base branch of
// `--upload-pack=touch <file> && git-upload-pack` made `git fetch origin
// <base>` run the command). Two independent layers now stop it:
// Prepare refuses the ref outright, and — should a value ever reach the
// argv unvalidated — --end-of-options turns it into a bad refspec.
func TestIntegrationBaseBranchCannotBecomeAGitOption(t *testing.T) {
	_, work := newFixtureRepo(t)
	marker := filepath.Join(t.TempDir(), "PWNED")
	payload := "--upload-pack=touch " + marker + " && git-upload-pack"
	g := New(exec.NewRunner(), 0)

	_, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: work, Base: payload, Branch: "feature/PROJ-1", Home: t.TempDir(), Key: "PROJ-1", RepoName: "acme.api",
	})
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "usage" {
		t.Errorf("Prepare with an option-shaped base: err = %v, want a usage error", err)
	}

	// The argv shape itself, bypassing validation.
	if _, err := g.run(context.Background(), work, "fetch", endOfOptions, "origin", payload); err == nil {
		t.Error("fetch of an option-shaped ref succeeded, want an invalid refspec failure")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the injected --upload-pack command ran")
	}
}
