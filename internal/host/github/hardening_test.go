package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// sequenceTransport answers the i-th request with responses[i] (the last
// one repeats), counting requests per method.
type sequenceTransport struct {
	responses []*http.Response
	methods   []string
}

func (s *sequenceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	i := min(len(s.methods), len(s.responses)-1)
	s.methods = append(s.methods, r.Method)
	resp := *s.responses[i]
	resp.Body = io.NopCloser(strings.NewReader(`{"message":"x"}`))
	return &resp, nil
}

func response(status int, header ...string) *http.Response {
	h := http.Header{}
	for i := 0; i+1 < len(header); i += 2 {
		h.Set(header[i], header[i+1])
	}
	return &http.Response{StatusCode: status, Header: h}
}

func noSleep(c *Client) *Client {
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

// TestSecondaryRateLimitIsRateLimitedNotAuth: GitHub signals its rate
// limits with 403; before phase 36 that reached the agent as host_auth
// — "your token is broken" — after no retry at all.
func TestSecondaryRateLimitIsRateLimitedNotAuth(t *testing.T) {
	for name, r := range map[string]*http.Response{
		"primary":   response(403, "X-RateLimit-Remaining", "0"),
		"secondary": response(403, "Retry-After", "1"),
	} {
		st := &sequenceTransport{responses: []*http.Response{r}}
		_, err := noSleep(New("tok", st, nil)).findOpenPR(context.Background(), "a", "b", "main", "f")
		var envErr *envelope.Error
		if !errors.As(err, &envErr) || envErr.Code != "rate_limited" {
			t.Errorf("%s: err = %v, want rate_limited", name, err)
		}
		if len(st.methods) != 3 {
			t.Errorf("%s: %d attempts, want 3", name, len(st.methods))
		}
	}

	st := &sequenceTransport{responses: []*http.Response{response(403)}}
	_, err := noSleep(New("tok", st, nil)).findOpenPR(context.Background(), "a", "b", "main", "f")
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "host_auth" || len(st.methods) != 1 {
		t.Errorf("plain 403: err = %v after %d attempts, want host_auth after 1", err, len(st.methods))
	}
}

// TestPRCreateIsNeverResentAfterGatewayError: a 502 on the POST that
// opens a pull request may have opened it; re-sending could open a
// second one. A transient 503 on the GET lookup before it is retried.
func TestPRCreateIsNeverResentAfterGatewayError(t *testing.T) {
	get := &sequenceTransport{responses: []*http.Response{response(503), response(200)}}
	if err := noSleep(New("tok", get, nil)).do(context.Background(), http.MethodGet, "/repos/a/b/pulls", nil, nil); err != nil || len(get.methods) != 2 {
		t.Errorf("GET after 503: err = %v, %d attempts; want success on the second", err, len(get.methods))
	}

	post := &sequenceTransport{responses: []*http.Response{response(502), response(201)}}
	err := noSleep(New("tok", post, nil)).do(context.Background(), http.MethodPost, "/repos/a/b/pulls", map[string]string{"title": "t"}, nil)
	if err == nil || len(post.methods) != 1 {
		t.Errorf("POST after 502: err = %v, %d attempts; want an error after exactly 1", err, len(post.methods))
	}
}
