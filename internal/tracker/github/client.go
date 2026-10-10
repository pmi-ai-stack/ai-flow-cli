package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/httpretry"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

var _ tracker.Tracker = (*Client)(nil)

// Client implements tracker.Tracker against GitHub Issues on github.com
// (base_url https://api.github.com) or GitHub Enterprise
// (https://<host>/api/v3).
type Client struct {
	baseURL    string
	authHeader string
	cfg        config.Tracker
	httpClient *http.Client
	timeout    time.Duration
	verbose    io.Writer
	// sleep waits between retry attempts; nil is a real sleep.
	sleep func(context.Context, time.Duration) error
}

// Limits, the same as the jira and host/github clients': a response
// over maxResponseBytes is an error, an error body is cut to
// errorBodyBytes before it reaches a message, and a list is read for
// at most maxPages pages of perPage.
const (
	maxResponseBytes = 32 << 20
	errorBodyBytes   = 2 << 10
	perPage          = 100
	maxPages         = 10
)

// DefaultTimeout bounds every request until WithTimeout says otherwise.
const DefaultTimeout = 30 * time.Second

// New builds a Client for cfg (provider "github"). transport and
// verbose follow jira.New's convention: the production caller passes
// http.DefaultTransport and a redacting writer, a test a fixture.
func New(cfg config.Tracker, token string, transport http.RoundTripper, verbose io.Writer) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(cfg.BaseURL, "/"),
		authHeader: "Bearer " + token,
		cfg:        cfg,
		httpClient: &http.Client{Transport: transport, Timeout: DefaultTimeout},
		timeout:    DefaultTimeout,
		verbose:    verbose,
	}
}

// WithTimeout sets the per-request deadline (non-positive keeps the
// current one) and returns c.
func (c *Client) WithTimeout(d time.Duration) *Client {
	if d > 0 {
		c.timeout = d
		c.httpClient.Timeout = d
	}
	return c
}

// issueRef is one ticket key resolved to its repository and number.
type issueRef struct {
	prefix string
	repo   string // owner/repo
	number string
}

func (r issueRef) path(suffix string) string {
	return "/repos/" + r.repo + "/issues/" + r.number + suffix
}

// resolve maps key to its issue through config.tracker.repositories. A
// prefix with no entry is config_invalid: the fix is configuration.
func (c *Client) resolve(key string) (issueRef, error) {
	i := strings.LastIndex(key, "-")
	if i <= 0 {
		return issueRef{}, &envelope.Error{Code: "usage", Message: fmt.Sprintf("github issues: %q is not a <PREFIX>-<number> key", key)}
	}
	prefix, number := key[:i], key[i+1:]
	repo, ok := c.cfg.Repositories[prefix]
	if !ok {
		return issueRef{}, &envelope.Error{
			Code:    "config_invalid",
			Message: fmt.Sprintf("tracker.repositories has no entry for key prefix %q (ticket %s)", prefix, key),
		}
	}
	return issueRef{prefix: prefix, repo: repo, number: number}, nil
}

// do sends one request through httpretry and decodes a 2xx body into
// out (nil to ignore it). A non-2xx response is an *envelope.Error.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("github issues: marshal request body: %w", err)
		}
		reqBody = data
	}
	rc := &httpretry.Client{HTTP: c.httpClient, Timeout: c.timeout, MaxBody: maxResponseBytes, Sleep: c.sleep}
	resp, err := rc.Do(ctx, func(ctx context.Context) (*http.Request, error) {
		var rb io.Reader
		if reqBody != nil {
			rb = bytes.NewReader(reqBody)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rb)
		if err != nil {
			return nil, fmt.Errorf("github issues: build request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.verbose != nil {
			fmt.Fprintf(c.verbose, "%s %s\n", method, path) //nolint:errcheck // best-effort diagnostic logging to --verbose's writer, never load-bearing
		}
		return req, nil
	})
	if err != nil {
		return httpretry.Failure(fmt.Sprintf("github issues: %s %s", method, path), err)
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return statusError(method, path, resp)
	}
	if out == nil || len(resp.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return fmt.Errorf("github issues: decode %s %s response: %w", method, path, err)
	}
	return nil
}

// statusError maps a non-2xx response to the tracker's exit-5 codes: a
// rate limit (429, or a 403 GitHub marks as one) is rate_limited, any
// other 401/403 tracker_auth, 404 tracker_not_found, the rest
// tool_failed.
func statusError(method, path string, resp httpretry.Response) error {
	code := "tool_failed"
	switch {
	case httpretry.IsRateLimited(resp):
		code = "rate_limited"
	case resp.Status == http.StatusUnauthorized, resp.Status == http.StatusForbidden:
		code = "tracker_auth"
	case resp.Status == http.StatusNotFound:
		code = "tracker_not_found"
	}
	return &envelope.Error{
		Code:    code,
		Message: fmt.Sprintf("github issues: %s %s: HTTP %d: %s", method, path, resp.Status, httpretry.Truncate(resp.Body, errorBodyBytes)),
	}
}

// pages GETs path page by page (perPage each, at most maxPages),
// decoding each into a fresh []T, and stops at the first short page.
func pages[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var all []T
	for page := 1; page <= maxPages; page++ {
		var batch []T
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s%sper_page=%d&page=%d", path, sep, perPage, page), nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	return all, nil
}
