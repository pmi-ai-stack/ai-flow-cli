package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/gitref"
)

// Validate checks cfg against every documented rule under the spec's
// Configuration section. Violations are aggregated with errors.Join —
// a caller wants every problem in one response, not one round trip per
// field (04-errors.md rule 8) — so the returned error, when non-nil, is
// always safe to pass straight through as *envelope.Error's Details.
//
// kubeconfigPath is the resolved kubeconfig file for the one
// warning-only rule (kube contexts exist); pass "" to skip that check
// entirely. Pass DefaultKubeconfigPath() in production.
func Validate(cfg *Config, kubeconfigPath string) (warnings []string, err error) {
	errs := []error{
		ruleSchemaVersion(cfg),
		ruleTeamsExist(cfg),
		ruleEveryEnvironmentHasTier(cfg),
		ruleTargetMapNamesConfiguredRepo(cfg),
		ruleRiskGateRange(cfg),
		ruleDefaultEnvironmentConfigured(cfg),
		ruleRepoSettingsRepoConfigured(cfg),
		ruleTierMultiplierPrecision(cfg),
		ruleE2ESandboxTier(cfg),
		ruleStandardsSourcesShape(cfg),
		ruleDefaultBaseBranchIsARef(cfg),
		ruleTrackerBaseURLSecure(cfg),
		ruleTrackerProvider(cfg),
	}
	return kubeconfigWarnings(cfg, kubeconfigPath), errors.Join(errs...)
}

// Rule 11 (re-asserted; Load already refuses to return a Config whose
// schema_version differs, so this only fires when Validate is called
// directly against a hand-built Config in a test).
func ruleSchemaVersion(cfg *Config) error {
	if cfg.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("schema_version: %d is not supported (want %d)", cfg.SchemaVersion, CurrentSchemaVersion)
	}
	return nil
}

// Rule 1: every team named in project_key_map exists in teams.
func ruleTeamsExist(cfg *Config) error {
	var errs []error
	for _, key := range sortedKeys(cfg.Routing.ProjectKeyMap) {
		team := cfg.Routing.ProjectKeyMap[key].Team
		if _, ok := cfg.Routing.Teams[team]; !ok {
			errs = append(errs, fmt.Errorf("routing.project_key_map.%s: team %q is not defined in routing.teams", key, team))
		}
	}
	return errors.Join(errs...)
}

// Rule 2: every environment has a tier, and it is one of the three
// documented values.
func ruleEveryEnvironmentHasTier(cfg *Config) error {
	var errs []error
	for _, name := range sortedKeys(cfg.Ops.Environments) {
		tier := cfg.Ops.Environments[name].Tier
		switch {
		case tier == "":
			errs = append(errs, fmt.Errorf("ops.environments.%s: missing tier", name))
		case !isValidTier(tier):
			errs = append(errs, fmt.Errorf("ops.environments.%s: tier %q is not one of sandbox, nonprod, prod", name, tier))
		}
	}
	return errors.Join(errs...)
}

func isValidTier(t string) bool {
	return t == "sandbox" || t == "nonprod" || t == "prod"
}

// Rule 3: every target_map key is <repo>:<path> naming a configured
// repository.
func ruleTargetMapNamesConfiguredRepo(cfg *Config) error {
	repos := configuredRepos(cfg)
	var errs []error
	for _, key := range sortedKeys(cfg.Ops.TargetMap) {
		repo, _, ok := strings.Cut(key, ":")
		if !ok || repo == "" {
			errs = append(errs, fmt.Errorf("ops.target_map: key %q is not shaped <repo>:<path>", key))
			continue
		}
		if !repos[repo] {
			errs = append(errs, fmt.Errorf("ops.target_map: key %q names repository %q, which is not configured in any routing.project_key_map entry", key, repo))
		}
	}
	return errors.Join(errs...)
}

// configuredRepos is every repository named by some
// routing.project_key_map entry — the only place config.json names
// repositories at all.
func configuredRepos(cfg *Config) map[string]bool {
	repos := make(map[string]bool)
	for _, entry := range cfg.Routing.ProjectKeyMap {
		for _, r := range entry.Repos {
			repos[r] = true
		}
	}
	return repos
}

// Rule 4: risk_gate is in [0, 100].
func ruleRiskGateRange(cfg *Config) error {
	if cfg.Ops.RiskGate < 0 || cfg.Ops.RiskGate > 100 {
		return fmt.Errorf("ops.risk_gate: %d is outside [0, 100]", cfg.Ops.RiskGate)
	}
	return nil
}

// Rule 5: default_environment names a configured environment.
func ruleDefaultEnvironmentConfigured(cfg *Config) error {
	if cfg.Ops.DefaultEnvironment == "" {
		return errors.New("ops.default_environment: must name a configured environment")
	}
	if _, ok := cfg.Ops.Environments[cfg.Ops.DefaultEnvironment]; !ok {
		return fmt.Errorf("ops.default_environment: %q is not defined in ops.environments", cfg.Ops.DefaultEnvironment)
	}
	return nil
}

// Rule 6: every repository in teams.<team>.repo_settings appears in
// some project_key_map entry.
func ruleRepoSettingsRepoConfigured(cfg *Config) error {
	repos := configuredRepos(cfg)
	var errs []error
	for _, teamName := range sortedKeys(cfg.Routing.Teams) {
		team := cfg.Routing.Teams[teamName]
		for _, repoName := range sortedKeys(team.RepoSettings) {
			if !repos[repoName] {
				errs = append(errs, fmt.Errorf("routing.teams.%s.repo_settings.%s: repository is not configured in any routing.project_key_map entry", teamName, repoName))
			}
		}
	}
	return errors.Join(errs...)
}

// Rule 7: every tier_multiplier value has at most three decimal places
// (it is stored per mille from phase 23 onward).
func ruleTierMultiplierPrecision(cfg *Config) error {
	var errs []error
	check := func(field string, v float64) {
		if !hasAtMostThreeDecimals(v) {
			errs = append(errs, fmt.Errorf("ops.risk.tier_multiplier.%s: %v has more than three decimal places", field, v))
		}
	}
	check("sandbox", cfg.Ops.Risk.TierMultiplier.Sandbox)
	check("nonprod", cfg.Ops.Risk.TierMultiplier.Nonprod)
	check("prod", cfg.Ops.Risk.TierMultiplier.Prod)
	return errors.Join(errs...)
}

func hasAtMostThreeDecimals(v float64) bool {
	const epsilon = 1e-6
	scaled := v * 1000
	return math.Abs(scaled-math.Round(scaled)) < epsilon
}

// Rule 9 (per the spec's own numbering; rule 8 is the kubeconfig
// warning, handled separately by kubeconfigWarnings): e2e's sandbox
// environment is sandbox-tier when e2e is enabled.
func ruleE2ESandboxTier(cfg *Config) error {
	if !cfg.Ops.E2E.Enabled {
		return nil
	}
	env, ok := cfg.Ops.Environments[cfg.Ops.E2E.SandboxEnvironment]
	if !ok {
		return fmt.Errorf("ops.e2e.sandbox_environment: %q is not defined in ops.environments", cfg.Ops.E2E.SandboxEnvironment)
	}
	if env.Tier != "sandbox" {
		return fmt.Errorf("ops.e2e.sandbox_environment: %q must be sandbox-tier when ops.e2e.enabled, has tier %q", cfg.Ops.E2E.SandboxEnvironment, env.Tier)
	}
	return nil
}

// Rule 12 (phase 34): host.default_base_branch is a valid branch name.
// It reaches `git fetch`/`checkout`/`diff` argv verbatim, and
// config.json is agent-writable between tickets — a value starting with
// "-" used to become a git option (`--upload-pack=<cmd>` ran <cmd>).
// internal/gitops also refuses it at the sink; this catches it at
// `config validate`, before any git command is attempted.
func ruleDefaultBaseBranchIsARef(cfg *Config) error {
	if err := gitref.Valid(cfg.Host.DefaultBaseBranch); err != nil {
		return fmt.Errorf("host.default_base_branch: %s", err.Error())
	}
	return nil
}

// Rule 13: tracker.base_url is an https URL (CheckTrackerBaseURL).
// Rule 14: tracker.provider is jira or github; github maps key
// prefixes to owner/repo in tracker.repositories, which jira refuses so
// that a setting never silently does nothing.
var (
	keyPrefixPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)
	ownerRepoPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)
)

func ruleTrackerProvider(cfg *Config) error {
	t := cfg.Tracker
	switch t.TrackerProvider() {
	case TrackerJira:
		if len(t.Repositories) > 0 {
			return fmt.Errorf("tracker.repositories: only used when tracker.provider is %q (it is %q)", TrackerGitHub, t.TrackerProvider())
		}
		return nil
	case TrackerGitHub:
	default:
		return fmt.Errorf("tracker.provider: %q is not one of %s, %s", t.Provider, TrackerJira, TrackerGitHub)
	}
	if len(t.Repositories) == 0 {
		return fmt.Errorf("tracker.repositories: must map at least one ticket-key prefix to owner/repo when tracker.provider is %q", TrackerGitHub)
	}
	var errs []error
	for _, prefix := range sortedKeys(t.Repositories) {
		repo := t.Repositories[prefix]
		if !keyPrefixPattern.MatchString(prefix) {
			errs = append(errs, fmt.Errorf("tracker.repositories.%s: the key must be a ticket-key prefix (uppercase letter, then uppercase letters or digits)", prefix))
		}
		if !ownerRepoPattern.MatchString(repo) {
			errs = append(errs, fmt.Errorf("tracker.repositories.%s: %q is not owner/repo", prefix, repo))
		}
	}
	return errors.Join(errs...)
}

func ruleTrackerBaseURLSecure(cfg *Config) error {
	return CheckTrackerBaseURL(cfg.Tracker.BaseURL)
}

// CheckTrackerBaseURL refuses a tracker base URL the Jira credentials
// must not be sent to: anything but an absolute https:// URL with a
// host, except plain http:// to a loopback host (localhost, 127.0.0.0/8,
// ::1), which is what a local fake tracker serves. Every request
// carries the Basic credentials, so a plain-http URL would put them on
// the wire in clear (2026-10-04 security review #3). Exported because
// it runs twice more outside `config validate` — before buildTracker
// and doctor's tracker probe attach any credential — since nothing
// forces `config validate` to run first.
func CheckTrackerBaseURL(raw string) error {
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		if u.Scheme == "https" || u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("tracker.base_url: %q must be an https:// URL (plain http:// is accepted only for a loopback host)", raw)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Rule 10: every standards source (org config.standards.sources, plus
// every team's own standards[]) has id, repo and ref; ids are unique
// across the org and team layers.
func ruleStandardsSourcesShape(cfg *Config) error {
	var errs []error
	seenAt := make(map[string]string)

	check := func(where string, s StandardsSource) {
		if s.ID == "" || s.Repo == "" || s.Ref == "" {
			errs = append(errs, fmt.Errorf("%s: standards source must have id, repo and ref", where))
			return
		}
		if firstWhere, dup := seenAt[s.ID]; dup {
			errs = append(errs, fmt.Errorf("%s: standards source id %q is already used at %s (ids must be unique across org and team layers)", where, s.ID, firstWhere))
			return
		}
		seenAt[s.ID] = where
	}

	for i, s := range cfg.Standards.Sources {
		check(fmt.Sprintf("standards.sources[%d]", i), s)
	}
	for _, teamName := range sortedKeys(cfg.Routing.Teams) {
		for i, s := range cfg.Routing.Teams[teamName].Standards {
			check(fmt.Sprintf("routing.teams.%s.standards[%d]", teamName, i), s)
		}
	}
	return errors.Join(errs...)
}

// Rule 8: every environments.*.kube.context exists in the kubeconfig at
// kubeconfigPath. Warning-only, per the spec's own "(warning)" — an
// unreadable kubeconfig is a warning too, never a validation failure.
// Skipped entirely (no warnings) when no environment names a kube
// context at all, or kubeconfigPath is "".
func kubeconfigWarnings(cfg *Config, kubeconfigPath string) []string {
	if kubeconfigPath == "" {
		return nil
	}

	var needed []string
	for _, name := range sortedKeys(cfg.Ops.Environments) {
		if env := cfg.Ops.Environments[name]; env.Kube != nil && env.Kube.Context != "" {
			needed = append(needed, env.Kube.Context)
		}
	}
	if len(needed) == 0 {
		return nil
	}

	contexts, err := kubeconfigContexts(kubeconfigPath)
	if err != nil {
		return []string{fmt.Sprintf("kubeconfig at %s is unreadable: %v", kubeconfigPath, err)}
	}
	have := make(map[string]bool, len(contexts))
	for _, c := range contexts {
		have[c] = true
	}

	var warnings []string
	for _, ctxName := range needed {
		if !have[ctxName] {
			warnings = append(warnings, fmt.Sprintf("kube context %q (named by an ops.environments entry) not found in kubeconfig at %s", ctxName, kubeconfigPath))
		}
	}
	return warnings
}

// sortedKeys returns m's keys in sorted order, so error and warning
// text built by iterating a map is deterministic (07-determinism.md).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
