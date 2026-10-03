package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

// countingTransport answers every request with status and body, and
// remembers each request's URL and Authorization header.
type countingTransport struct {
	status int
	body   string
	urls   []string
	auths  []string
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.urls = append(c.urls, r.URL.String())
	c.auths = append(c.auths, r.Header.Get("Authorization"))
	return &http.Response{StatusCode: c.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(c.body))}, nil
}

// TestAttachmentOnAnotherHostIsNeverRequested is SEC-12: an attachment
// URL from the issue payload pointing outside tracker.base_url is
// refused before any request is built — the Jira credentials never
// leave for that host.
func TestAttachmentOnAnotherHostIsNeverRequested(t *testing.T) {
	for _, u := range []string{
		"https://attacker.example/steal",
		"http://example.atlassian.net/secure/attachment/1/x", // scheme downgrade
		"https://example.atlassian.net.attacker.example/x",
		"https://example.atlassian.net:8443/x",
	} {
		ct := &countingTransport{status: 200, body: "data"}
		c := New(testTrackerConfig(0), "a@b.com", "tok", ct, nil)
		_, err := c.DownloadAttachments(context.Background(), &tracker.Issue{
			Attachments: []tracker.Attachment{{Filename: "x", URL: u}},
		}, t.TempDir())
		var envErr *envelope.Error
		if !errors.As(err, &envErr) || envErr.Code != "tool_failed" {
			t.Errorf("%s: err = %v, want tool_failed", u, err)
		}
		if len(ct.urls) != 0 {
			t.Errorf("%s: %d request(s) sent (with Authorization %q), want none", u, len(ct.urls), ct.auths)
		}
	}

	ct := &countingTransport{status: 200, body: "data"}
	c := New(testTrackerConfig(0), "a@b.com", "tok", ct, nil)
	if _, err := c.DownloadAttachments(context.Background(), &tracker.Issue{
		Attachments: []tracker.Attachment{{Filename: "x", URL: "https://EXAMPLE.atlassian.net/secure/attachment/1/x"}},
	}, t.TempDir()); err != nil || len(ct.urls) != 1 {
		t.Errorf("same host (any case): err = %v, %d requests; want one fetch", err, len(ct.urls))
	}
}

func TestOversizedResponseIsAnError(t *testing.T) {
	ct := &countingTransport{status: 200, body: strings.Repeat("x", maxResponseBytes+1)}
	c := New(testTrackerConfig(0), "a@b.com", "tok", ct, nil)
	if _, err := c.Statuses(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("oversized response: err = %v, want a size error", err)
	}
}

func TestErrorBodyIsTruncatedInMessages(t *testing.T) {
	ct := &countingTransport{status: 500, body: strings.Repeat("e", 10*errorBodyBytes)}
	c := New(testTrackerConfig(0), "a@b.com", "tok", ct, nil)
	_, err := c.Statuses(context.Background())
	if err == nil || len(err.Error()) > errorBodyBytes+200 {
		t.Errorf("error message is %d bytes, want the body cut to %d", len(err.Error()), errorBodyBytes)
	}
}

// A comment POST hit by a gateway error is not re-sent: it may already
// have been applied.
func TestCommentPostNotRetriedOnGatewayError(t *testing.T) {
	ct := &countingTransport{status: 502, body: "bad gateway"}
	c := New(testTrackerConfig(0), "a@b.com", "tok", ct, nil)
	recordSleeps(c)
	if err := c.postComment(context.Background(), "TEST-1", "hello"); err == nil {
		t.Fatal("postComment on 502 = nil error")
	}
	if len(ct.urls) != 1 {
		t.Errorf("POST was sent %d times, want 1", len(ct.urls))
	}
}
