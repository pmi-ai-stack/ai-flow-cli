package ops

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
)

// Ref names which git ref a Render call materializes: the branch under
// review, or the merge base of the branch and the configured base
// branch (phase 19's "base", via internal/gitops's own merge-base
// worktree).
type Ref string

const (
	RefBranch Ref = "branch"
	RefBase   Ref = "base"
)

// TreeSkipper is an optional interface an Addon implements when its
// own Render copies a target's source tree with a directory-skip rule
// (terraform's .terraform/, any dot-directory). internal/ops/render's
// TreeHash applies the same rule, so content the rendering can never
// include — a local `terraform init`'s provider binaries, a root-level
// target's .git/ — neither costs a read nor moves the cache key.
// Optional, so the Addon interface itself stays frozen (phase 18); an
// addon that reads its source in place (kustomize, github-actions,
// jenkins) does not implement it and every file stays hashed.
type TreeSkipper interface {
	// SkipTreeDir reports whether a directory with this base name, below
	// the target root, is never part of a rendering's input.
	SkipTreeDir(name string) bool
}

// Addon is the spec's own tool-independent interface (§ Addon
// interface and capability matrix). No implementation exists before
// phase 21 (terraform); this phase only declares the shape so
// Registry, ops detect and the fixture addon (used by phases 19/20/23's
// own tests) have something to compile against.
//
// Detect takes detect.Repo directly — the same shape every
// internal/detect function and internal/cli's own repo.json helpers
// already use for "one repository's local checkout" — rather than a
// redeclared type, per the phase-18 plan's own reuse rule.
type Addon interface {
	Name() string
	Capabilities() Capabilities
	// Detect returns every target this addon finds under repo's own
	// checkout. changed is nil at scope "repo" (no diff exists yet);
	// otherwise it is repo.json's in-scope files, filtered to those
	// under repo's own local path. Detect does not set Target.Repo,
	// Target.Scope or Target.Changed — DetectAll fills those in.
	Detect(repo detect.Repo, changed []string) ([]Target, error)
	Render(ctx context.Context, t Target, ref Ref, out string) (*Rendered, error)
	Validate(ctx context.Context, r *Rendered) ([]Finding, error)
	Drift(ctx context.Context, t Target, env Env, baseRender *Rendered) (*DriftReport, error)
	DryRun(ctx context.Context, t Target, env Env, r *Rendered) (*Changeset, error)
	Rollback(ctx context.Context, t Target, env Env, cs *Changeset) (*RollbackInfo, error)
	Harnesses(repo detect.Repo, t Target) ([]Harness, error)
}
