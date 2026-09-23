package ops

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
