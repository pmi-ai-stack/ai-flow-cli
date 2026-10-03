package jira

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

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
// same auth header and status-code mapping.
func (c *Client) getBytes(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jira: build attachment request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)

	if c.verbose != nil {
		fmt.Fprintf(c.verbose, "GET %s\n", rawURL) //nolint:errcheck // best-effort diagnostic logging to --verbose's writer, never load-bearing
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira: GET %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jira: read attachment body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, statusError(http.MethodGet, rawURL, resp.StatusCode, body)
	}
	return body, nil
}
