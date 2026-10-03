package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

// DownloadAttachments implements tracker.Tracker.DownloadAttachments.
func (c *Client) DownloadAttachments(ctx context.Context, issue *tracker.Issue, dir string) ([]string, error) {
	written := make([]string, 0, len(issue.Attachments))
	for _, a := range issue.Attachments {
		data, err := c.getBytes(ctx, a.URL)
		if err != nil {
			return nil, err
		}
		filename := filepath.Base(a.Filename)
		if _, err := handoff.WriteFile(dir, filename, data); err != nil {
			return nil, fmt.Errorf("jira: write attachment %s: %w", filename, err)
		}
		written = append(written, filename)
	}
	return written, nil
}

// getBytes fetches rawURL's raw content — an attachment's content URL,
// already absolute — bypassing do's JSON decode step while reusing the
// same auth header, retry loop and status-code mapping.
//
// rawURL comes from the issue payload, not from configuration, so it is
// fetched only when its scheme and host are tracker.base_url's own
// (SEC-12): the request carries the Jira credentials, and an attachment
// pointing anywhere else would hand them to that host. The check runs
// before any request is built.
func (c *Client) getBytes(ctx context.Context, rawURL string) ([]byte, error) {
	if err := c.sameOrigin(rawURL); err != nil {
		return nil, err
	}
	resp, err := c.send(ctx, maxAttachmentBytes, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("jira: build attachment request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader)
		if c.verbose != nil {
			fmt.Fprintf(c.verbose, "GET %s\n", rawURL) //nolint:errcheck // best-effort diagnostic logging to --verbose's writer, never load-bearing
		}
		return req, nil
	})
	if err != nil {
		return nil, fmt.Errorf("jira: GET %s: %w", rawURL, err)
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, statusError(http.MethodGet, rawURL, resp.Status, resp.Body)
	}
	return resp.Body, nil
}

// sameOrigin refuses rawURL unless its scheme and host (with port)
// equal tracker.base_url's.
func (c *Client) sameOrigin(rawURL string) error {
	u, err := url.Parse(rawURL)
	base, baseErr := url.Parse(c.baseURL)
	if err != nil || baseErr != nil || u.Scheme != base.Scheme || !strings.EqualFold(u.Host, base.Host) {
		return &envelope.Error{
			Code:    "tool_failed",
			Message: fmt.Sprintf("jira: attachment URL %q is not on the configured tracker %q; not fetched, so the Jira credentials are not sent to it", rawURL, c.baseURL),
		}
	}
	return nil
}
