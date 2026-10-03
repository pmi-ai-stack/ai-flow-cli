// Package httpretry is the one request loop the tracker and host
// clients share: a fresh request per attempt, each under its own
// deadline, a bounded body read, and retries only where a retry cannot
// apply a write twice.
package httpretry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// MaxAttempts bounds every call: the first try plus two retries.
const MaxAttempts = 3

// MaxWait is the longest a server-requested wait (Retry-After) is slept
// through; a longer one is returned to the caller as a rate limit at
// once rather than stalling an agent turn.
const MaxWait = 30 * time.Second

// Response is one completed exchange.
type Response struct {
	Status int
	Header http.Header
	Body   []byte

	method string // the request's, for RetryAfter's idempotency rule
}

// Client sends requests. Sleep waits between attempts; nil means a real,
// context-aware sleep (tests inject a recorder).
type Client struct {
	HTTP    *http.Client
	Timeout time.Duration
	MaxBody int64
	Sleep   func(context.Context, time.Duration) error
}

// ErrTooLarge is returned (wrapped) when a body exceeds MaxBody.
type ErrTooLarge struct{ Limit int64 }

func (e *ErrTooLarge) Error() string {
	return fmt.Sprintf("response body exceeds %d bytes", e.Limit)
}

// Do runs newReq (called once per attempt, so a request body is always
// re-sendable) and returns the final response. A non-2xx status is a
// Response, not an error: the caller maps it. err is a transport
// failure, a deadline, or ErrTooLarge — none of which is retried.
func (c *Client) Do(ctx context.Context, newReq func(context.Context) (*http.Request, error)) (Response, error) {
	var last Response
	for attempt := 1; ; attempt++ {
		resp, err := c.once(ctx, newReq)
		if err != nil {
			return Response{}, err
		}
		last = resp
		wait, retry := RetryAfter(resp)
		if !retry || attempt == MaxAttempts || wait > MaxWait {
			return last, nil
		}
		if wait == 0 {
			wait = time.Duration(attempt) * time.Second // 1 s, then 2 s
		}
		if err := c.sleep(ctx, wait); err != nil {
			return last, nil //nolint:nilerr // the context ended mid-backoff: report the response we have, not the cancellation
		}
	}
}

func (c *Client) once(ctx context.Context, newReq func(context.Context) (*http.Request, error)) (Response, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	req, err := newReq(ctx)
	if err != nil {
		return Response{}, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var r io.Reader = resp.Body
	if c.MaxBody > 0 {
		r = io.LimitReader(resp.Body, c.MaxBody+1)
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return Response{}, err
	}
	if c.MaxBody > 0 && int64(len(body)) > c.MaxBody {
		return Response{}, &ErrTooLarge{Limit: c.MaxBody}
	}
	return Response{Status: resp.StatusCode, Header: resp.Header, Body: body, method: req.Method}, nil
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsRateLimited reports whether r is a rate-limit refusal: 429, or a
// 403 GitHub marks as one (x-ratelimit-remaining: 0, or a Retry-After —
// its secondary rate limit). Nothing was applied by such a request.
func IsRateLimited(r Response) bool {
	if r.Status == http.StatusTooManyRequests {
		return true
	}
	return r.Status == http.StatusForbidden &&
		(r.Header.Get("X-RateLimit-Remaining") == "0" || r.Header.Get("Retry-After") != "")
}

// RetryAfter decides whether r should be retried and how long the
// server asked to wait (0 = no preference). Rate limits retry for any
// method; 502/503/504 only for idempotent methods, because a gateway
// error does not say whether a POST was applied — re-sending `pr
// create` or a comment would duplicate it.
func RetryAfter(r Response) (time.Duration, bool) {
	switch {
	case IsRateLimited(r):
	case r.Status == http.StatusServiceUnavailable && r.Header.Get("Retry-After") != "":
		// 503 with Retry-After: the server says it did not process it.
	case r.Status == http.StatusBadGateway, r.Status == http.StatusServiceUnavailable, r.Status == http.StatusGatewayTimeout:
		if !idempotent(r.method) {
			return 0, false
		}
	default:
		return 0, false
	}
	return parseRetryAfter(r.Header), true
}

func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	}
	return false
}

// parseRetryAfter reads Retry-After as delta-seconds or an HTTP date,
// else GitHub's x-ratelimit-reset (epoch seconds) when remaining is 0.
func parseRetryAfter(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s >= 0 {
			return time.Duration(s) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			return max(time.Until(t), 0)
		}
	}
	if h.Get("X-RateLimit-Remaining") == "0" {
		if s, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return max(time.Until(time.Unix(s, 0)), 0)
		}
	}
	return 0
}

// Truncate is body as an error message carries it: at most n bytes,
// marked when cut.
func Truncate(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + fmt.Sprintf("… (%d more bytes)", len(body)-n)
}
