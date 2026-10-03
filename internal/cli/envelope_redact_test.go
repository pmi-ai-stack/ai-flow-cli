package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

func useEnvelopeEnviron(t *testing.T, environ ...string) {
	t.Helper()
	orig := envelopeRedactor
	envelopeRedactor = func() *redact.Redactor { return redact.New(environ) }
	t.Cleanup(func() { envelopeRedactor = orig })
}

// TestEnvelopeIsRedacted is SEC-10, reproduced with the built binary
// first: a token pasted into config.json came straight back out of
// `config validate`'s error. Every envelope — JSON and --human, success
// and failure — now goes through one redacting funnel.
func TestEnvelopeIsRedacted(t *testing.T) {
	const token = "not-a-real-jira-value"
	useEnvelopeEnviron(t, "AI_FLOW_JIRA_TOKEN="+token)

	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	path := filepath.Join(home, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["ops"].(map[string]any)["target_map"] = map[string]any{token: "prod"}
	data, _ = json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"config", "validate", "--home", home},
		{"config", "validate", "--home", home, "--human"},
	} {
		stdout, _, ec := runRoot(t, args...)
		if ec == 0 {
			t.Fatalf("%v: want a validation failure naming the bad target_map key", args)
		}
		if strings.Contains(stdout.String(), token) {
			t.Errorf("%v printed the token:\n%s", args, stdout.String())
		}
		if !strings.Contains(stdout.String(), redact.HashPrefix(token)) {
			t.Errorf("%v: want the redaction token in place of the value:\n%s", args, stdout.String())
		}
	}
}

// TestEnvelopeRedactionLeavesNonSecretsAlone is the improvement plan's
// 3.5 → 3.6 dependency as a regression test: a profile name that
// happens to equal a value in the envelope (a development build's
// "dev"; here the fixture build's commit) is not a secret and must
// survive — before 3.5 it would have been redacted.
func TestEnvelopeRedactionLeavesNonSecretsAlone(t *testing.T) {
	useEnvelopeEnviron(t, "AWS_PROFILE="+fixedBuildInfo.Commit, "AWS_REGION="+fixedBuildInfo.Version)
	stdout, _, ec := runRoot(t, "version")
	out := stdout.String()
	if ec != 0 || !strings.Contains(out, `"commit": "`+fixedBuildInfo.Commit+`"`) || !strings.Contains(out, `"version": "`+fixedBuildInfo.Version+`"`) {
		t.Errorf("version with AWS_PROFILE/AWS_REGION set to envelope values: exitCode = %d\n%s", ec, out)
	}
}
