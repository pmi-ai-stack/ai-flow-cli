package jira

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// authCapturingTransport records the Authorization header of the one
// request it handles, then defers to a wrapped fixtureTransport for the
// response.
type authCapturingTransport struct {
	inner http.RoundTripper
	got   string
}

func (a *authCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	a.got = req.Header.Get("Authorization")
	return a.inner.RoundTrip(req)
}

func TestClient_AuthHeaderIsBasic(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/issue/TEST-1?fields="+issueFields, http.StatusOK, "issue-no-attachments.json")
	auth := &authCapturingTransport{inner: ft}

	c := New(testTrackerConfig(0), "user@example.com", "s3cr3t", auth, nil)
	if _, err := c.Fetch(context.Background(), "TEST-1"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	const want = "Basic dXNlckBleGFtcGxlLmNvbTpzM2NyM3Q=" // base64("user@example.com:s3cr3t")
	if auth.got != want {
		t.Fatalf("Authorization header = %q, want %q", auth.got, want)
	}
}

func statusMappingCase(t *testing.T, status int, file, wantCode string) {
	t.Helper()
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/issue/TEST-1?fields="+issueFields, status, file)

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	slept := recordSleeps(c)
	_, err := c.Fetch(context.Background(), "TEST-1")
	if wantCode == "rate_limited" && len(*slept) != 2 {
		t.Errorf("rate limit: %d backoffs, want 2 (three attempts)", len(*slept))
	}

	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("expected *envelope.Error, got %v", err)
	}
	if envErr.Code != wantCode {
		t.Fatalf("error.code = %q, want %q", envErr.Code, wantCode)
	}
}

func TestClient_StatusCodeMapping(t *testing.T) {
	statusMappingCase(t, http.StatusUnauthorized, "error-401.txt", "tracker_auth")
	statusMappingCase(t, http.StatusForbidden, "error-401.txt", "tracker_auth")
	statusMappingCase(t, http.StatusNotFound, "error-404.txt", "tracker_not_found")
	statusMappingCase(t, http.StatusTooManyRequests, "error-429.txt", "rate_limited")
	statusMappingCase(t, http.StatusInternalServerError, "error-500-with-token.txt", "tool_failed")
}
