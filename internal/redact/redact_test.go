package redact

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// fakeEnviron builds an os.Environ()-shaped slice from a map, for tests
// that must not touch the real process environment.
func fakeEnviron(kv map[string]string) []string {
	out := make([]string, 0, len(kv))
	for k, v := range kv {
		out = append(out, k+"="+v)
	}
	return out
}

// TestRedactorCorpus is the phase's required redaction corpus test: a
// token discovered from the environment appears in no output, across
// every shape this package's methods produce.
func TestRedactorCorpus(t *testing.T) {
	const (
		jiraToken = "jira-secret-token-9f8e7d6c"
		awsKey    = "AKIAABCDEFGHIJKLMNOP"
		nonSecret = "not-a-secret-value"
	)
	environ := fakeEnviron(map[string]string{
		"AI_FLOW_JIRA_TOKEN":    jiraToken,
		"AWS_SECRET_ACCESS_KEY": awsKey,
		"SOME_OTHER_VAR":        nonSecret,
	})
	r := New(environ)

	logLine := "level=INFO msg=\"calling jira\" token=" + jiraToken + " note=" + nonSecret
	jsonBlob, err := json.Marshal(map[string]string{
		"note":   "using " + awsKey + " to sign the request",
		"filler": nonSecret,
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	errMsg := "tracker_auth: request failed with token " + jiraToken + " (" + nonSecret + ")"

	outputs := map[string]string{
		"log line":  r.String(logLine),
		"json blob": string(r.Bytes(jsonBlob)),
		"error msg": r.String(errMsg),
	}

	for name, out := range outputs {
		if strings.Contains(out, jiraToken) {
			t.Errorf("%s: jira token leaked: %q", name, out)
		}
		if strings.Contains(out, awsKey) {
			t.Errorf("%s: aws key leaked: %q", name, out)
		}
		if !strings.Contains(out, "<redacted:") {
			t.Errorf("%s: no redaction token present: %q", name, out)
		}
		if !strings.Contains(out, nonSecret) {
			t.Errorf("%s: non-secret value was redacted away: %q", name, out)
		}
	}

	// Writer: the same corpus, through the streaming wrapper.
	var buf bytes.Buffer
	w := r.Writer(&buf)
	n, err := w.Write([]byte(logLine))
	if err != nil {
		t.Fatalf("Writer.Write: %v", err)
	}
	if n != len(logLine) {
		t.Errorf("Writer.Write returned n=%d, want len(p)=%d", n, len(logLine))
	}
	if strings.Contains(buf.String(), jiraToken) {
		t.Errorf("Writer: jira token leaked: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "<redacted:") {
		t.Errorf("Writer: no redaction token present: %q", buf.String())
	}
}

func TestNonSecretEnvVarsUntouched(t *testing.T) {
	environ := fakeEnviron(map[string]string{
		"AI_FLOW_FIXTURE_DIR": "/tmp/fixtures/PROJ-123",
		"TF_PLUGIN_CACHE_DIR": "/home/user/.ai-flow/cache/terraform",
	})
	r := New(environ)
	if len(r.secrets) != 0 {
		t.Fatalf("New() discovered %d secrets from explicitly non-secret vars, want 0: %+v", len(r.secrets), r.secrets)
	}

	text := "fixture dir is /tmp/fixtures/PROJ-123, cache is /home/user/.ai-flow/cache/terraform"
	if got := r.String(text); got != text {
		t.Errorf("String() modified text with only non-secret vars in scope: got %q, want unchanged", got)
	}
}

func TestSecretDiscoveryTable(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		wantFound bool
	}{
		{"AI_FLOW_JIRA_TOKEN", "abcdef", true},
		{"AI_FLOW_JIRA_EMAIL", "person@example.com", true},
		{"AI_FLOW_GITHUB_TOKEN", "ghp_abcdef", true},
		{"GITHUB_TOKEN", "ghp_abcdef", true},
		{"GH_TOKEN", "ghp_abcdef", true},
		{"KUBECONFIG", "/home/user/.kube/config", true},
		{"GOOGLE_APPLICATION_CREDENTIALS", "/home/user/creds.json", true},
		{"AI_FLOW_JENKINS_USER", "svc-account", true},
		{"AI_FLOW_JENKINS_TOKEN", "abcdef", true},
		{"ANTHROPIC_API_KEY", "sk-ant-abcdef", true},
		{"AWS_ACCESS_KEY_ID", "AKIAABCDEFGHIJKLMNOP", true},
		{"AWS_SECRET_ACCESS_KEY", "abcdefghijklmnop", true},
		{"ARM_CLIENT_SECRET", "abcdefghijklmnop", true},
		{"AI_FLOW_FIXTURE_DIR", "/tmp/fixtures", false},
		{"TF_PLUGIN_CACHE_DIR", "/tmp/cache", false},
		{"HOME", "/home/user", false},
		{"PATH", "/usr/bin:/bin", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := New(fakeEnviron(map[string]string{c.name: c.value}))
			found := len(r.secrets) == 1
			if found != c.wantFound {
				t.Errorf("isSecretName(%q) via New = %v, want %v", c.name, found, c.wantFound)
			}
		})
	}
}

func TestShortValuesNotTreatedAsSecrets(t *testing.T) {
	r := New(fakeEnviron(map[string]string{"AI_FLOW_JIRA_TOKEN": "ab"}))
	if len(r.secrets) != 0 {
		t.Errorf("New() with a 2-byte value discovered %d secrets, want 0 (below minSecretLen)", len(r.secrets))
	}
}

func TestHashPrefixStable(t *testing.T) {
	a := HashPrefix("same-value")
	b := HashPrefix("same-value")
	if a != b {
		t.Errorf("HashPrefix(same value) = %q, %q — want identical", a, b)
	}
	c := HashPrefix("different-value")
	if a == c {
		t.Errorf("HashPrefix(different values) both = %q — want different tokens", a)
	}
	if !strings.HasPrefix(a, "<redacted:") || !strings.HasSuffix(a, ">") {
		t.Errorf("HashPrefix format = %q, want <redacted:...>", a)
	}
}

func TestRedactMarked(t *testing.T) {
	text := `{"data": "super-secret-value", "other": "kept"}`
	marked := []MarkedValue{{Pointer: "/data", Value: "super-secret-value"}}

	got := RedactMarked(text, marked)
	if strings.Contains(got, "super-secret-value") {
		t.Errorf("RedactMarked: value leaked: %q", got)
	}
	if !strings.Contains(got, "<redacted:") {
		t.Errorf("RedactMarked: no token present: %q", got)
	}
	if !strings.Contains(got, "kept") {
		t.Errorf("RedactMarked: unmarked value was removed: %q", got)
	}
}

func TestRedactMarkedNoEnvInvolved(t *testing.T) {
	// RedactMarked works with zero Redactor/environment involvement —
	// the marking-driven path phase 19 needs before render() ever
	// builds a Redactor.
	got := RedactMarked("value is X", []MarkedValue{{Pointer: "/x", Value: "X"}})
	if strings.Contains(got, "value is X") {
		t.Errorf("RedactMarked did not redact: %q", got)
	}
}
