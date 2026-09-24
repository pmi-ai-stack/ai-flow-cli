package jenkins

import (
	"os"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// resolveBinding returns env's own Jenkins URL, or
// ops.ErrLiveCredentialsMissing when name is unbound ("") or bound to
// an environment with no jenkins binding configured — the identical
// shape terraform's/kubectl's own resolveBinding already establish,
// used by Drift and DryRun (both receive an ops.Env).
func resolveBinding(cfg *config.Config, name string) (string, error) {
	if name == "" {
		return "", ops.ErrLiveCredentialsMissing
	}
	env, ok := cfg.Ops.Environments[name]
	if !ok || env.Jenkins == nil {
		return "", ops.ErrLiveCredentialsMissing
	}
	return env.Jenkins.URL, nil
}

// jenkinsBinding resolves the one Jenkins server Validate itself
// reaches for, independent of any target's own bound environment —
// Addon.Validate's own frozen signature receives no Env (see doc.go's
// own paragraph on "Jenkins CLI reachability"). The first
// cfg.Ops.Environments entry, sorted by name, whose own Jenkins
// binding is non-nil; ok is false when none exists anywhere in
// config, in which case Validate degrades to a soft finding rather
// than failing a ticket that never configured a Jenkins environment
// at all.
func jenkinsBinding(cfg *config.Config) (url string, ok bool) {
	if cfg == nil {
		return "", false
	}
	names := make([]string, 0, len(cfg.Ops.Environments))
	for name := range cfg.Ops.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if b := cfg.Ops.Environments[name].Jenkins; b != nil {
			return b.URL, true
		}
	}
	return "", false
}

// jenkinsAuthArgs builds jenkins-cli's own "-auth user:token" argument
// pair from AI_FLOW_JENKINS_USER/AI_FLOW_JENKINS_TOKEN, read directly
// from the environment (never through config.Config) — the same
// precedent internal/cli/ticket.go's buildTracker already established
// for Jira's own credentials. Empty when either is unset: some Jenkins
// servers allow anonymous read access, and a missing credential should
// not stop this addon from at least attempting the call — a real
// server that requires auth then fails cleanly as tool_failed.
func jenkinsAuthArgs() []string {
	user := os.Getenv("AI_FLOW_JENKINS_USER")
	token := os.Getenv("AI_FLOW_JENKINS_TOKEN")
	if user == "" || token == "" {
		return nil
	}
	return []string{"-auth", user + ":" + token}
}
