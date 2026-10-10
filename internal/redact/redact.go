package redact

import (
	"slices"
	"strings"
)

// secretEnvVars is every environment variable the spec's "Secrets and
// live access" table names whose *value* is a credential. Excluded:
// AI_FLOW_FIXTURE_DIR and TF_PLUGIN_CACHE_DIR (the table documents both
// as non-secret), and — since phase 34 — KUBECONFIG and
// GOOGLE_APPLICATION_CREDENTIALS, whose values are paths to credential
// files, not credentials; redacting them only scrubbed a file path from
// every message naming it. AWS_ and ARM_ are prefixes, minus the
// configuration variables in nonSecretUnderPrefix; every other row is
// an exact variable name.
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
		"AI_FLOW_JENKINS_USER",
		"AI_FLOW_JENKINS_TOKEN",
		"JENKINS_API_TOKEN",
		"ANTHROPIC_API_KEY",
	},
	prefixes: []string{"AWS_", "ARM_"},
}

// nonSecretUnderPrefix is the configuration the AWS and Azure SDKs read
// from AWS_*/ARM_* variables that is not a credential — regions,
// profile names, endpoints, file paths, feature switches and public
// identifiers. Classifying these by prefix alone turned "planning in
// eu-central-1 … with profile prod-admin" into three redaction tokens,
// corrupting the very terraform output an agent has to reason about.
// The list is closed on purpose: an AWS_/ARM_ variable not named here
// is still a secret — unknown means secret. Names ending in "*" match
// as a prefix (AWS_ENDPOINT_URL_<SERVICE>, ARM_USE_<METHOD>).
var nonSecretUnderPrefix = []string{
	"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_DEFAULT_OUTPUT",
	"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CA_BUNDLE",
	"AWS_SDK_LOAD_CONFIG", "AWS_PAGER", "AWS_RETRY_MODE", "AWS_MAX_ATTEMPTS", "AWS_STS_REGIONAL_ENDPOINTS",
	"AWS_EC2_METADATA_DISABLED", "AWS_USE_FIPS_ENDPOINT", "AWS_USE_DUALSTACK_ENDPOINT", "AWS_ENDPOINT_URL*",
	"ARM_SUBSCRIPTION_ID", "ARM_TENANT_ID", "ARM_CLIENT_ID", "ARM_ENVIRONMENT", "ARM_METADATA_HOSTNAME",
	"ARM_USE_*", "ARM_SKIP_PROVIDER_REGISTRATION", "ARM_STORAGE_USE_AZUREAD",
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
			return !isNonSecretSetting(name)
		}
	}
	return false
}

func isNonSecretSetting(name string) bool {
	for _, n := range nonSecretUnderPrefix {
		if p, ok := strings.CutSuffix(n, "*"); ok {
			if strings.HasPrefix(name, p) {
				return true
			}
		} else if name == n {
			return true
		}
	}
	return false
}
