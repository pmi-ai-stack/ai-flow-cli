package config

import "time"

// Config is the full ~/.ai-flow/config.json shape, field for field
// against the spec's own example under "Configuration and data model".
// json tags match the example byte for byte (10-data-types.md); an
// unknown field in the file is a decode error (see Load), not a
// silently-ignored one.
type Config struct {
	SchemaVersion int       `json:"schema_version"`
	Tracker       Tracker   `json:"tracker"`
	Host          Host      `json:"host"`
	Routing       Routing   `json:"routing"`
	Defaults      Defaults  `json:"defaults"`
	Standards     Standards `json:"standards"`
	Ops           Ops       `json:"ops"`
}

// Tracker is config.tracker.
type Tracker struct {
	Provider     string          `json:"provider"`
	BaseURL      string          `json:"base_url"`
	Statuses     TrackerStatuses `json:"statuses"`
	BugTypes     []string        `json:"bug_types"`
	CommentLimit int             `json:"comment_limit"`
}

// TrackerStatuses is config.tracker.statuses: the tracker's own status
// names for each class ai-flow reasons about.
type TrackerStatuses struct {
	Todo       []string `json:"todo"`
	InProgress string   `json:"in_progress"`
	InReview   string   `json:"in_review"`
}

// Host is config.host.
type Host struct {
	Provider          string `json:"provider"`
	DefaultBaseBranch string `json:"default_base_branch"`
	DraftPRs          bool   `json:"draft_prs"`
}

// Routing is config.routing: the project-key/keyword/team layers Load
// keeps in file order (map iteration order is never relied on outside
// this package; any artifact written from it sorts first).
type Routing struct {
	ProjectKeyMap map[string]ProjectKeyMapEntry `json:"project_key_map"`
	KeywordToRepo map[string][]string           `json:"keyword_to_repo"`
	Teams         map[string]Team               `json:"teams"`
}

// ProjectKeyMapEntry is one config.routing.project_key_map entry.
type ProjectKeyMapEntry struct {
	Team  string   `json:"team"`
	Repos []string `json:"repos"`
}

// Team is one config.routing.teams entry.
type Team struct {
	LocalBase    string                  `json:"local_base"`
	RepoSettings map[string]RepoSettings `json:"repo_settings"`
	Standards    []StandardsSource       `json:"standards"`
}

// RepoSettings is one config.routing.teams.<team>.repo_settings entry —
// per-repository overrides of a default.
type RepoSettings struct {
	MutationThreshold int `json:"mutation_threshold"`
}

// StandardsSource is one pack source: config.standards.sources, or a
// team's own config.routing.teams.<team>.standards.
type StandardsSource struct {
	ID   string `json:"id"`
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
}

// Defaults is config.defaults.
type Defaults struct {
	Worktree               bool   `json:"worktree"`
	Viewport               string `json:"viewport"`
	MutationThreshold      int    `json:"mutation_threshold"`
	MutationMaxRounds      int    `json:"mutation_max_rounds"`
	MutationTimeoutMinutes int    `json:"mutation_timeout_minutes"`
	TestTimeoutMinutes     int    `json:"test_timeout_minutes"`
	// GuardIdleHours defaults to 8 when the field is absent or zero —
	// see Load's applyDefaults. A config file that means "never idle"
	// has no way to spell that with an integer field; zero is not a
	// meaningful value for this field, so treating it as "unset" costs
	// nothing real.
	GuardIdleHours int `json:"guard_idle_hours"`
}

// MutationTimeout is MutationTimeoutMinutes as a time.Duration — call
// sites never re-parse a literal (10-data-types.md).
func (d Defaults) MutationTimeout() time.Duration {
	return time.Duration(d.MutationTimeoutMinutes) * time.Minute
}

// TestTimeout is TestTimeoutMinutes as a time.Duration.
func (d Defaults) TestTimeout() time.Duration {
	return time.Duration(d.TestTimeoutMinutes) * time.Minute
}

// ResolveMutationThreshold returns repo's effective mutation score
// threshold: team's own repo_settings override when one is configured
// (a zero MutationThreshold means "not set" — 10-data-types.md's own
// convention for an unset integer field, same as GuardIdleHours), else
// cfg.Defaults.MutationThreshold. Matches the spec's own example
// exactly: "Team Beta"'s repo_settings names 90 for
// acme.billing.api specifically, while every other repository falls
// through to defaults.mutation_threshold (80 there).
func ResolveMutationThreshold(cfg *Config, team, repo string) int {
	if t, ok := cfg.Routing.Teams[team]; ok {
		if rs, ok := t.RepoSettings[repo]; ok && rs.MutationThreshold != 0 {
			return rs.MutationThreshold
		}
	}
	return cfg.Defaults.MutationThreshold
}

// GuardIdleDuration is GuardIdleHours as a time.Duration — what the
// guard (phase 9) actually compares an idle active.json entry against.
func (d Defaults) GuardIdleDuration() time.Duration {
	return time.Duration(d.GuardIdleHours) * time.Hour
}

// Standards is config.standards: the org layer's sources and the
// guidance token budget.
type Standards struct {
	Sources              []StandardsSource `json:"sources"`
	GuidanceBudgetTokens int               `json:"guidance_budget_tokens"`
}

// Ops is config.ops.
type Ops struct {
	Environments       map[string]Environment `json:"environments"`
	TargetMap          map[string]string      `json:"target_map"`
	DefaultEnvironment string                 `json:"default_environment"`
	RiskGate           int                    `json:"risk_gate"`
	Risk               Risk                   `json:"risk"`
	ProtectedKinds     []string               `json:"protected_kinds"`
	E2E                E2E                    `json:"e2e"`
	ToolTimeoutMinutes int                    `json:"tool_timeout_minutes"`
}

// ToolTimeout is ToolTimeoutMinutes as a time.Duration.
func (o Ops) ToolTimeout() time.Duration {
	return time.Duration(o.ToolTimeoutMinutes) * time.Minute
}

// Environment is one config.ops.environments entry: a named live target
// with a tier and per-addon bindings. An addon's binding block is nil
// when the environment has no counterpart for it.
type Environment struct {
	Tier      string            `json:"tier"`
	Kube      *KubeBinding      `json:"kube,omitempty"`
	Terraform *TerraformBinding `json:"terraform,omitempty"`
	Jenkins   *JenkinsBinding   `json:"jenkins,omitempty"`
}

// KubeBinding is an environment's kubectl/helm/kustomize binding.
type KubeBinding struct {
	Context string `json:"context"`
}

// TerraformBinding is an environment's terraform binding.
type TerraformBinding struct {
	Workspace string `json:"workspace"`
	VarFile   string `json:"var_file"`
}

// JenkinsBinding is an environment's Jenkins binding.
type JenkinsBinding struct {
	URL string `json:"url"`
}

// Risk is config.ops.risk: the weights and multipliers risk scoring
// (phase 23) reads. Values here are the user-authored floats from the
// config file; phase 23's internal per-mille integer arithmetic is a
// separate, later concern (see Determinism's float-free rule) — this
// package only validates that tier_multiplier is precise enough to
// convert cleanly.
type Risk struct {
	ActionWeight   ActionWeight   `json:"action_weight"`
	KindWeight     KindWeight     `json:"kind_weight"`
	TierMultiplier TierMultiplier `json:"tier_multiplier"`
	DriftPenalty   int            `json:"drift_penalty"`
	OfflinePenalty int            `json:"offline_penalty"`
}

// ActionWeight is config.ops.risk.action_weight.
type ActionWeight struct {
	Create  int `json:"create"`
	Update  int `json:"update"`
	Replace int `json:"replace"`
	Delete  int `json:"delete"`
	Noop    int `json:"noop"`
}

// KindWeight is config.ops.risk.kind_weight, one weight per kind class.
type KindWeight struct {
	Workload       int `json:"workload"`
	Network        int `json:"network"`
	RBAC           int `json:"rbac"`
	Secret         int `json:"secret"`
	Data           int `json:"data"`
	IAM            int `json:"iam"`
	CRDOrNamespace int `json:"crd_or_namespace"`
	Pipeline       int `json:"pipeline"`
	Other          int `json:"other"`
}

// TierMultiplier is config.ops.risk.tier_multiplier: a per-tier
// multiplier, at most three decimal places (stored per mille once
// phase 23 converts it).
type TierMultiplier struct {
	Sandbox float64 `json:"sandbox"`
	Nonprod float64 `json:"nonprod"`
	Prod    float64 `json:"prod"`
}

// E2E is config.ops.e2e.
type E2E struct {
	Enabled            bool   `json:"enabled"`
	SandboxEnvironment string `json:"sandbox_environment"`
}
