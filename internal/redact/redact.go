package redact

import (
	"slices"
	"strings"
)

// secretEnvVars is every environment variable the spec's "Secrets and
// live access" table names, except AI_FLOW_FIXTURE_DIR and
// TF_PLUGIN_CACHE_DIR — that table documents both as non-secret (a
// fixture-swap path and a cache directory the CLI itself sets, not a
// credential). AWS_ and ARM_ are prefixes, exactly as the table lists
// them; every other row is an exact variable name.
var secretEnvVars = struct {
	exact    []string
	prefixes []string
}{
	exact: []string{
		"AI_FLOW_JIRA_EMAIL",
		"AI_FLOW_JIRA_TOKEN",
		"AI_FLOW_GITHUB_TOKEN",
		"GITHUB_TOKEN",
		"GH_TOKEN",
		"KUBECONFIG",
		"GOOGLE_APPLICATION_CREDENTIALS",
		"AI_FLOW_JENKINS_USER",
		"AI_FLOW_JENKINS_TOKEN",
		"ANTHROPIC_API_KEY",
	},
	prefixes: []string{"AWS_", "ARM_"},
}

// minSecretLen is the shortest value New treats as a secret. Shorter
// values are almost always empty or a placeholder; redacting them would
// replace common short substrings everywhere instead of protecting
// anything.
const minSecretLen = 3

// Secret is one environment-variable secret New discovered: its name
// (kept for diagnostics and tests, never printed in any artifact) and
// value (what gets redacted).
type Secret struct {
	Name  string
	Value string
}

// Redactor replaces every discovered secret's value with a stable,
// irreversible token wherever it appears in text passed to its methods.
// The zero value (no secrets) is valid and redacts nothing.
type Redactor struct {
	secrets []Secret
}

// New discovers secrets from environ — each entry shaped "KEY=VALUE",
// exactly what os.Environ() returns. Pass os.Environ() in production, a
// fixed slice in tests; this package never reads the process
// environment on its own, so a test never needs a real secret to prove
// the mechanism.
func New(environ []string) *Redactor {
	var secrets []Secret
	for _, kv := range environ {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || len(value) < minSecretLen {
			continue
		}
		if isSecretName(name) {
			secrets = append(secrets, Secret{Name: name, Value: value})
		}
	}
	return &Redactor{secrets: secrets}
}

func isSecretName(name string) bool {
	if slices.Contains(secretEnvVars.exact, name) {
		return true
	}
	for _, prefix := range secretEnvVars.prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
