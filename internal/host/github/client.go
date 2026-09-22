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

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/host"
)

const defaultBaseURL = "https://api.github.com"

// Client implements host.Host against a GitHub Cloud or Enterprise
// instance over its REST API v3.
var _ host.Host = (*Client)(nil)

type Client struct {
	baseURL    string
	authHeader string
	httpClient *http.Client
	verbose    io.Writer
}

// New builds a Client. baseURL is defaultBaseURL for github.com; a
// caller pointing at GitHub Enterprise passes its own API base.
// transport and verbose follow jira.New's exact convention (see that
// package's own doc comment).
func New(token string, transport http.RoundTripper, verbose io.Writer) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		authHeader: "Bearer " + token,
		httpClient: &http.Client{Transport: transport},
		verbose:    verbose,
	}
}

// do sends one request: method against c.baseURL+path, body marshaled
// as the JSON request body (nil for none), the response decoded into
// out. A non-2xx response is mapped to *envelope.Error via statusError.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("github: marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.verbose != nil {
		fmt.Fprintf(c.verbose, "%s %s\n", method, path) //nolint:errcheck // best-effort diagnostic logging to --verbose's writer, never load-bearing
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("github: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(method, path, resp.StatusCode, respBody)
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
// error.code (design decision 13): 401/403 an auth failure, 429 a rate
// limit, everything else (404, 422, ...) a generic tool failure — the
// repository, branch or request not being what was asked for, not
// GitHub declining to authenticate the caller.
func statusError(method, path string, status int, body []byte) error {
	code := "tool_failed"
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		code = "host_auth"
	case http.StatusTooManyRequests:
		code = "rate_limited"
	}
	return &envelope.Error{
		Code:    code,
		Message: fmt.Sprintf("github: %s %s: HTTP %d: %s", method, path, status, string(body)),
	}
}
