package jira

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// TestErrorMessageRedactable proves the CLI-layer redaction pass this
// phase relies on (internal/cli's redactErr, applied to every error a
// tracker.Tracker call returns) actually removes a leaked token: a
// Jira 500 response whose body happens to contain the literal token
// value produces an *envelope.Error whose Message still contains it
// verbatim (this package never redacts on its own — design decision
// 3, "never touches os.Getenv") — but once passed through
// redact.New(environ).String, exactly as the CLI layer does, the token
// is gone.
func TestErrorMessageRedactable(t *testing.T) {
	const token = "ai-flow-secret-jira-token-value"

	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/issue/TEST-1?fields="+issueFields, http.StatusInternalServerError, "error-500-with-token.txt")

	c := New(testTrackerConfig(0), "a@b.com", token, ft, nil)
	_, err := c.Fetch(context.Background(), "TEST-1")

	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("expected *envelope.Error, got %v", err)
	}
	if !strings.Contains(envErr.Message, token) {
		t.Fatalf("fixture setup is broken: expected the raw error to contain the token so the redaction step is actually exercised")
	}

	redacted := redact.New([]string{"AI_FLOW_JIRA_TOKEN=" + token}).String(envErr.Message)
	if strings.Contains(redacted, token) {
		t.Fatalf("token still present after redaction: %q", redacted)
	}
}
