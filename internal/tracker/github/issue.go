package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

type ghLabel struct {
	Name string `json:"name"`
}

type ghIssue struct {
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	Body        *string         `json:"body"`
	State       string          `json:"state"`
	Labels      []ghLabel       `json:"labels"`
	Type        *ghIssueType    `json:"type"`
	PullRequest json.RawMessage `json:"pull_request"`
}

type ghIssueType struct {
	Name string `json:"name"`
}

func (i ghIssue) labelNames() []string {
	out := make([]string, len(i.Labels))
	for k, l := range i.Labels {
		out[k] = l.Name
	}
	return out
}

// getIssue reads ref's issue. The issues API also serves pull requests
// (with a pull_request field); a ticket must be an issue.
func (c *Client) getIssue(ctx context.Context, ref issueRef, key string) (ghIssue, error) {
	var wire ghIssue
	if err := c.do(ctx, http.MethodGet, ref.path(""), nil, &wire); err != nil {
		return ghIssue{}, err
	}
	if len(wire.PullRequest) > 0 && string(wire.PullRequest) != "null" {
		return ghIssue{}, &envelope.Error{
			Code:    "tracker_not_found",
			Message: fmt.Sprintf("github issues: %s is a pull request in %s, not an issue", key, ref.repo),
		}
	}
	return wire, nil
}

// Fetch implements tracker.Tracker.Fetch.
func (c *Client) Fetch(ctx context.Context, key string) (*tracker.Issue, error) {
	ref, err := c.resolve(key)
	if err != nil {
		return nil, err
	}
	wire, err := c.getIssue(ctx, ref, key)
	if err != nil {
		return nil, err
	}
	labels := wire.labelNames()
	typeName := ""
	if wire.Type != nil {
		typeName = wire.Type.Name
	}
	issue := &tracker.Issue{
		Key:         key,
		Project:     ref.prefix,
		IssueType:   issueTypeOf(c.cfg, typeName, labels),
		Summary:     wire.Title,
		Status:      statusOf(c.cfg, wire.State, labels),
		Attachments: []tracker.Attachment{},
	}
	if wire.Body != nil {
		issue.Description = *wire.Body
	}

	if c.cfg.CommentLimit > 0 {
		comments, err := c.listComments(ctx, ref)
		if err != nil {
			return nil, err
		}
		if len(comments) > c.cfg.CommentLimit {
			comments = comments[len(comments)-c.cfg.CommentLimit:]
		}
		for _, gc := range comments {
			issue.Comments = append(issue.Comments, tracker.Comment{Author: gc.User.Login, Body: gc.Body, Created: gc.CreatedAt})
		}
	}
	return issue, nil
}

// CurrentStatus implements tracker.Tracker.CurrentStatus.
func (c *Client) CurrentStatus(ctx context.Context, key string) (string, error) {
	ref, err := c.resolve(key)
	if err != nil {
		return "", err
	}
	wire, err := c.getIssue(ctx, ref, key)
	if err != nil {
		return "", err
	}
	return statusOf(c.cfg, wire.State, wire.labelNames()), nil
}

// Transition implements tracker.Tracker.Transition: add the target
// label, then remove every other configured status label the issue
// carries. Nothing is written when it already has exactly the target.
func (c *Client) Transition(ctx context.Context, key string, class tracker.StatusClass) error {
	target, ok := tracker.TargetStatusName(c.cfg, class)
	if !ok {
		return &envelope.Error{Code: "usage", Message: fmt.Sprintf("github issues: %q is not a valid transition target", class)}
	}
	ref, err := c.resolve(key)
	if err != nil {
		return err
	}
	wire, err := c.getIssue(ctx, ref, key)
	if err != nil {
		return err
	}
	labels := wire.labelNames()
	var stale []string
	for _, l := range statusLabels(c.cfg) {
		if l != target && slices.Contains(labels, l) {
			stale = append(stale, l)
		}
	}
	slices.Sort(stale)
	stale = slices.Compact(stale)

	if !slices.Contains(labels, target) {
		if err := c.do(ctx, http.MethodPost, ref.path("/labels"), map[string][]string{"labels": {target}}, nil); err != nil {
			return err
		}
	}
	for _, l := range stale {
		// A label removed since the read (404) is already the state we want.
		if err := c.do(ctx, http.MethodDelete, ref.path("/labels/"+url.PathEscape(l)), nil, nil); err != nil && !isNotFound(err) {
			return err
		}
	}
	return nil
}

// DownloadAttachments implements tracker.Tracker.DownloadAttachments.
// GitHub has no attachment API: there is never anything to write.
func (c *Client) DownloadAttachments(context.Context, *tracker.Issue, string) ([]string, error) {
	return []string{}, nil
}
