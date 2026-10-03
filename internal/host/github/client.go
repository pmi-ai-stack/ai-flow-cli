// Package github implements host.Host against the GitHub REST API v3.
// Mirrors internal/tracker/jira's shape exactly: a Client built from an
// already-resolved token (never os.Getenv itself, so its own tests need
// no real environment or credentials) and an injectable
// http.RoundTripper (production: http.DefaultTransport; tests: a
// recorded-fixture transport).
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/host"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/httpretry"
)

const defaultBaseURL = "https://api.github.com"

// Client implements host.Host against a GitHub Cloud or Enterprise
// instance over its REST API v3.
var _ host.Host = (*Client)(nil)

type Client struct {
	baseURL    string
	authHeader string
	httpClient *http.Client
	timeout    time.Duration
	verbose    io.Writer
	// sleep waits between retry attempts; nil is a real sleep.
	sleep func(context.Context, time.Duration) error
}

// maxResponseBytes bounds every response read; errorBodyBytes is how
// much of an error body reaches a message (jira's limits, same reason).
const (
	maxResponseBytes = 32 << 20
	errorBodyBytes   = 2 << 10
)

// DefaultTimeout bounds every request until WithTimeout says otherwise —
// jira.DefaultTimeout's exact convention.
const DefaultTimeout = 30 * time.Second

// New builds a Client. baseURL is defaultBaseURL for github.com; a
// caller pointing at GitHub Enterprise passes its own API base.
// transport and verbose follow jira.New's exact convention (see that
// package's own doc comment).
func New(token string, transport http.RoundTripper, verbose io.Writer) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		authHeader: "Bearer " + token,
		httpClient: &http.Client{Transport: transport, Timeout: DefaultTimeout},
		timeout:    DefaultTimeout,
		verbose:    verbose,
	}
}

// WithTimeout sets the per-request deadline (non-positive keeps the
// current one) and returns c — jira.Client.WithTimeout's exact
// convention, applied as both http.Client.Timeout and a per-request
// context deadline.
func (c *Client) WithTimeout(d time.Duration) *Client {
	if d > 0 {
		c.timeout = d
		c.httpClient.Timeout = d
	}
	return c
}

// do sends one request: method against c.baseURL+path, body marshaled
// as the JSON request body (nil for none), the response decoded into
// out. A non-2xx response is mapped to *envelope.Error via statusError.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("github: marshal request body: %w", err)
		}
		reqBody = data
	}

	// httpretry: a fresh request per attempt under c.timeout, bounded
	// body, rate limits retried for any method and gateway errors only
	// for idempotent ones — `pr create`'s POST is never re-sent after a
	// 502, which could open a second PR (PERF-10).
	rc := &httpretry.Client{HTTP: c.httpClient, Timeout: c.timeout, MaxBody: maxResponseBytes, Sleep: c.sleep}
	resp, err := rc.Do(ctx, func(ctx context.Context) (*http.Request, error) {
		var rb io.Reader
		if reqBody != nil {
			rb = bytes.NewReader(reqBody)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rb)
		if err != nil {
			return nil, fmt.Errorf("github: build request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader)
		req.Header.Set("Accept", "application/vnd.github+json")
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.verbose != nil {
			fmt.Fprintf(c.verbose, "%s %s\n", method, path) //nolint:errcheck // best-effort diagnostic logging to --verbose's writer, never load-bearing
		}
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	respBody := resp.Body

	if resp.Status < 200 || resp.Status >= 300 {
		return statusError(method, path, resp, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("github: decode %s %s response: %w", method, path, err)
	}
	return nil
}

// statusError maps a non-2xx GitHub response to a documented exit-5
// error.code (design decision 13): a rate limit — 429, or the 403
// GitHub uses for its primary (x-ratelimit-remaining: 0) and secondary
// (Retry-After) limits — is rate_limited, never host_auth, which used
// to tell the agent a working token was broken; any other 401/403 an
// auth failure; everything else (404, 422, ...) a generic tool failure.
func statusError(method, path string, resp httpretry.Response, body []byte) error {
	code := "tool_failed"
	switch {
	case httpretry.IsRateLimited(resp):
		code = "rate_limited"
	case resp.Status == http.StatusUnauthorized, resp.Status == http.StatusForbidden:
		code = "host_auth"
	}
	return &envelope.Error{
		Code:    code,
		Message: fmt.Sprintf("github: %s %s: HTTP %d: %s", method, path, resp.Status, httpretry.Truncate(body, errorBodyBytes)),
	}
}
