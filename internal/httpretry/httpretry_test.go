package httpretry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// scripted answers each request with the next status (and headers),
// recording every request it saw.
type scripted struct {
	statuses []int
	headers  []http.Header
	body     string
	seen     []string
}

func (s *scripted) RoundTrip(r *http.Request) (*http.Response, error) {
	i := len(s.seen)
	var b []byte
	if r.Body != nil {
		b, _ = io.ReadAll(r.Body)
	}
	s.seen = append(s.seen, r.Method+" "+string(b))
	h := http.Header{}
	if i < len(s.headers) && s.headers[i] != nil {
		h = s.headers[i]
	}
	return &http.Response{StatusCode: s.statuses[min(i, len(s.statuses)-1)], Header: h, Body: io.NopCloser(strings.NewReader(s.body))}, nil
}

func run(t *testing.T, method string, s *scripted, maxBody int64) ([]time.Duration, Response, error) {
	t.Helper()
	var slept []time.Duration
	c := &Client{
		HTTP: &http.Client{Transport: s}, Timeout: time.Second, MaxBody: maxBody,
		Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}
	resp, err := c.Do(context.Background(), func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, method, "https://api.example.com/x", strings.NewReader("payload"))
	})
	return slept, resp, err
}

func TestRetryIdempotentOnGatewayErrors(t *testing.T) {
	s := &scripted{statuses: []int{503, 502, 200}}
	slept, resp, err := run(t, http.MethodGet, s, 0)
	if err != nil || resp.Status != 200 || len(s.seen) != 3 {
		t.Fatalf("GET 503,502,200: status %d err %v after %d requests", resp.Status, err, len(s.seen))
	}
	if !slices.Equal(slept, []time.Duration{time.Second, 2 * time.Second}) {
		t.Errorf("backoff = %v, want [1s 2s]", slept)
	}
	if s.seen[2] != "GET payload" {
		t.Errorf("third attempt sent %q, want the body re-sent", s.seen[2])
	}
}

// A POST after a gateway error may already have been applied: never
// re-sent (a duplicate PR or comment is worse than a failure).
func TestNoRetryForPostOnGatewayError(t *testing.T) {
	for _, status := range []int{502, 504, 503} {
		s := &scripted{statuses: []int{status, 200}}
		_, resp, _ := run(t, http.MethodPost, s, 0)
		if resp.Status != status || len(s.seen) != 1 {
			t.Errorf("POST %d: %d requests, final status %d; want exactly one", status, len(s.seen), resp.Status)
		}
	}
}

// A rate limit is a refusal — nothing was applied — so even a POST is
// retried, honouring Retry-After.
func TestRateLimitRetriesAnyMethodHonouringRetryAfter(t *testing.T) {
	s := &scripted{statuses: []int{429, 403, 201}, headers: []http.Header{
		{"Retry-After": []string{"7"}},
		{"X-Ratelimit-Remaining": []string{"0"}, "Retry-After": []string{"3"}},
	}}
	slept, resp, _ := run(t, http.MethodPost, s, 0)
	if resp.Status != 201 || len(s.seen) != 3 {
		t.Fatalf("POST 429,403(rate),201: %d requests, status %d", len(s.seen), resp.Status)
	}
	if !slices.Equal(slept, []time.Duration{7 * time.Second, 3 * time.Second}) {
		t.Errorf("slept %v, want [7s 3s]", slept)
	}
}

func TestGiveUpAfterMaxAttemptsOrLongWait(t *testing.T) {
	s := &scripted{statuses: []int{429}}
	slept, resp, _ := run(t, http.MethodGet, s, 0)
	if resp.Status != 429 || len(s.seen) != MaxAttempts || len(slept) != MaxAttempts-1 {
		t.Errorf("persistent 429: %d requests, %d sleeps; want %d and %d", len(s.seen), len(slept), MaxAttempts, MaxAttempts-1)
	}

	long := &scripted{statuses: []int{429, 200}, headers: []http.Header{{"Retry-After": []string{"3600"}}}}
	slept, resp, _ = run(t, http.MethodGet, long, 0)
	if resp.Status != 429 || len(long.seen) != 1 || len(slept) != 0 {
		t.Errorf("Retry-After 3600: %d requests, slept %v; want an immediate rate limit", len(long.seen), slept)
	}
}

func TestPlain403AndOtherErrorsAreNotRetried(t *testing.T) {
	for _, status := range []int{401, 403, 404, 422, 500} {
		s := &scripted{statuses: []int{status, 200}}
		if _, resp, _ := run(t, http.MethodGet, s, 0); resp.Status != status || len(s.seen) != 1 {
			t.Errorf("GET %d was retried", status)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	_, _, err := run(t, http.MethodGet, &scripted{statuses: []int{200}, body: strings.Repeat("x", 11)}, 10)
	var tooLarge *ErrTooLarge
	if !errors.As(err, &tooLarge) || tooLarge.Limit != 10 {
		t.Errorf("11-byte body with a 10-byte limit: err = %v, want ErrTooLarge", err)
	}
	if _, resp, err := run(t, http.MethodGet, &scripted{statuses: []int{200}, body: strings.Repeat("x", 10)}, 10); err != nil || len(resp.Body) != 10 {
		t.Errorf("10-byte body at the limit: %v, %d bytes", err, len(resp.Body))
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate([]byte("abcdef"), 3); got != "abc… (3 more bytes)" {
		t.Errorf("Truncate = %q", got)
	}
	if got := Truncate([]byte("abc"), 3); got != "abc" {
		t.Errorf("Truncate at the limit = %q", got)
	}
}
