package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/httpretry"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

// Client implements tracker.Tracker against a Jira Cloud or Server
// instance over its REST API v2.
var _ tracker.Tracker = (*Client)(nil)

type Client struct {
	baseURL    string
	authHeader string
	cfg        config.Tracker
	httpClient *http.Client
	timeout    time.Duration
	verbose    io.Writer
	// sleep waits between retry attempts; nil is a real sleep. Tests
	// set it so retries cost no wall time.
	sleep func(context.Context, time.Duration) error
}

// Size limits on what is read from Jira (SEC-12): a response or
// attachment over the limit is an error, never a silent truncation;
// an error body is cut to errorBodyBytes before it reaches a message.
const (
	maxResponseBytes   = 32 << 20
	maxAttachmentBytes = 100 << 20
	errorBodyBytes     = 2 << 10
)

// DefaultTimeout bounds every request a Client built by New makes until
// WithTimeout says otherwise — the production caller passes
// config.defaults.http_timeout_seconds; this default keeps any other
// caller from ever being unbounded.
const DefaultTimeout = 30 * time.Second

// New builds a Client. transport is the http.RoundTripper the
// production caller sets to http.DefaultTransport and a test sets to a
// recorded-fixture transport. verbose, when non-nil, receives one
// "<method> <path>" line per request — the caller wraps it in
// redact.New(os.Environ()).Writer before passing it in (--verbose:
// "tool logs to stderr, secrets redacted").
func New(cfg config.Tracker, email, token string, transport http.RoundTripper, verbose io.Writer) *Client {
	creds := base64.StdEncoding.EncodeToString([]byte(email + ":" + token))
	return &Client{
		baseURL:    cfg.BaseURL,
		authHeader: "Basic " + creds,
		cfg:        cfg,
		httpClient: &http.Client{Transport: transport, Timeout: DefaultTimeout},
		timeout:    DefaultTimeout,
		verbose:    verbose,
	}
}

// WithTimeout sets the per-request deadline (non-positive keeps the
// current one) and returns c. It is applied twice, deliberately:
// http.Client.Timeout covers dial through body read on the default
// transport, and a per-request context deadline covers a RoundTripper
// that does not honour Client.Timeout.
func (c *Client) WithTimeout(d time.Duration) *Client {
	if d > 0 {
		c.timeout = d
		c.httpClient.Timeout = d
	}
	return c
}

// do sends one request: method against c.baseURL+path, body marshaled
// as the JSON request body (nil for none), the response decoded into
// out (nil when the caller expects no meaningful body, e.g. a 204). A
// non-2xx response is mapped to *envelope.Error via statusError.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("jira: marshal request body: %w", err)
		}
		reqBody = data
	}

	resp, err := c.send(ctx, maxResponseBytes, func(ctx context.Context) (*http.Request, error) {
		var body io.Reader
		if reqBody != nil {
			body = bytes.NewReader(reqBody)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return nil, fmt.Errorf("jira: build request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader)
		req.Header.Set("Accept", "application/json")
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.verbose != nil {
			fmt.Fprintf(c.verbose, "%s %s\n", method, path) //nolint:errcheck // best-effort diagnostic logging to --verbose's writer, never load-bearing
		}
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("jira: %s %s: %w", method, path, err)
	}
	respBody := resp.Body

	if resp.Status < 200 || resp.Status >= 300 {
		return statusError(method, path, resp.Status, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("jira: decode %s %s response: %w", method, path, err)
	}
	return nil
}

// send runs one logical request through httpretry: a fresh request per
// attempt (newReq), each attempt under c.timeout, at most maxBody bytes
// read, rate limits and idempotent gateway errors retried (PERF-10).
func (c *Client) send(ctx context.Context, maxBody int64, newReq func(context.Context) (*http.Request, error)) (httpretry.Response, error) {
	rc := &httpretry.Client{HTTP: c.httpClient, Timeout: c.timeout, MaxBody: maxBody, Sleep: c.sleep}
	return rc.Do(ctx, newReq)
}

// statusError maps a non-2xx Jira response to a documented exit-5
// error.code: 401/403 are an auth failure, 404 is "no such issue" or
// "no such comment", 429 is a rate limit, and everything else is a
// generic tool failure — Jira's own workflow or configuration not
// doing what was asked, not one of the three more specific codes.
func statusError(method, path string, status int, body []byte) error {
	code := "tool_failed"
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		code = "tracker_auth"
	case http.StatusNotFound:
		code = "tracker_not_found"
	case http.StatusTooManyRequests:
		code = "rate_limited"
	}
	return &envelope.Error{
		Code:    code,
		Message: fmt.Sprintf("jira: %s %s: HTTP %d: %s", method, path, status, httpretry.Truncate(body, errorBodyBytes)),
	}
}
