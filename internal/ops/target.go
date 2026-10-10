package ops

import (
	"errors"
	"path/filepath"
	"strings"
)

// Target is one deployable unit an addon found: a Terraform root
// module, a Helm chart + values file, a Kustomize overlay, a directory
// of raw manifests, a workflow file, a Jenkins job. This is
// ops-targets.json's own per-target shape (schema/ops-targets.schema.json);
// struct fields are declared in the JSON-key alphabetical order
// 07-determinism.md requires (internal/handoff's canonical marshaller
// does not sort struct fields itself — only map keys — so the source
// order here is the byte-stability guarantee).
//
// Repo is not named among the spec's own "addon, path, scope, changed,
// environment, tier, files" list (§ OPS: targets, changeset, stages,
// addons), but config.ops.target_map's own documented key shape is
// "<repo>:<path>" (the spec's config example: "acme.k8s-config:overlays/eu-prod").
// Binding an environment needs the repo name as a first-class value,
// not something parsed back out of a path string that never had it
// encoded — see the phase-18 plan's design decision 1.
type Target struct {
	Addon string `json:"addon"`
	// Changed is nil while Scope is "repo" (no diff exists yet); at
	// Scope "diff" it is true when Files is non-empty for this target.
	Changed *bool `json:"changed"`
	// Environment is the bound environment's name, or nil when the
	// addon has no live counterpart at all (the unbound environment).
	Environment *string `json:"environment"`
	// Files is every changed file (relative to the repository root)
	// this target claims — empty while Scope is "repo".
	Files []string `json:"files"`
	// Path is relative to Repo's own local checkout root.
	Path string `json:"path"`
	Repo string `json:"repo"`
	// Scope is "repo" at `plan` (nothing changed yet) or "diff" from
	// `implement` onward.
	Scope string `json:"scope"`
	// Tier is the bound environment's tier, or "nonprod" when
	// Environment is nil (the unbound environment's own tier).
	Tier *string `json:"tier"`
}

const (
	ScopeRepo = "repo"
	ScopeDiff = "diff"
)

// TargetKey is a target's canonical string identity: "<repo>:<path>" —
// config.ops.target_map's own documented key shape (the spec's config
// example: "acme.k8s-config:overlays/eu-prod"), already reused by
// internal/ops/envs.Bind. Used to match `--target T` on every
// per-target OPS command from phase 19 on, and as the "target" string
// phase 20's render-drift-results.json/dryrun-results.json record.
func TargetKey(t Target) string {
	return t.Repo + ":" + t.Path
}

// CheckTarget refuses a target whose Repo, Addon or Path could escape
// the directory it is joined under: the redacted rendering lives at
// <KEY>/render/<repo>/<addon>/<path>/<hash>/ and a target's source is
// <checkout>/<path>. Repo and Addon must be names (not empty, "." or a
// path that leaves its parent); Path must be relative and stay inside
// the repository ("." is the repository root). Detection never
// produces anything this refuses, so a refusal means ops-targets.json
// was edited or a detector is broken (security review 2026-09-27 S10).
// The message is the reason only; callers prefix TargetKey.
func CheckTarget(t Target) error {
	name := func(s string) bool {
		return filepath.IsLocal(filepath.FromSlash(s)) && filepath.Clean(filepath.FromSlash(s)) != "."
	}
	switch {
	case !name(t.Repo):
		return errors.New("repo must be a name, not a path")
	case !name(t.Addon):
		return errors.New("addon must be a name, not a path")
	case !filepath.IsLocal(filepath.FromSlash(t.Path)):
		return errors.New("path must be relative and stay inside the repository")
	}
	return nil
}

// FilesUnderPath returns every entry of changed that equals path or
// lies under path+"/" — a prefix match (terraform's own convention),
// shared by every addon whose own targets never nest a second,
// independent target beneath a first (a chart's/overlay's own changed
// content routinely lives in a subdirectory of the target's own root —
// "templates/", "patches/" — which a direct-child match would silently
// drop from Target.Files). kubectl's own raw-manifests directories can
// legitimately nest, so that addon keeps its own direct-child match
// rather than calling this.
func FilesUnderPath(path string, changed []string) []string {
	var files []string
	prefix := path + "/"
	for _, f := range changed {
		if f == path || strings.HasPrefix(f, prefix) {
			files = append(files, f)
		}
	}
	return files
}
