package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

type jiraStatus struct {
	Name string `json:"name"`
}

type jiraIssueType struct {
	Name string `json:"name"`
}

type jiraProject struct {
	Key string `json:"key"`
}

type jiraAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type jiraIssueFields struct {
	Summary     string           `json:"summary"`
	Description string           `json:"description"`
	Status      jiraStatus       `json:"status"`
	IssueType   jiraIssueType    `json:"issuetype"`
	Project     jiraProject      `json:"project"`
	Attachment  []jiraAttachment `json:"attachment"`
}

type jiraIssue struct {
	Key    string          `json:"key"`
	Fields jiraIssueFields `json:"fields"`
}

// issueFields is the field list requested on every issue GET — kept
// explicit and minimal so fixture payloads (and real ones) stay small
// and deterministic rather than the full, much larger default field
// set Jira returns with no ?fields filter.
const issueFields = "summary,description,status,issuetype,project,attachment"

// Fetch implements tracker.Tracker.Fetch.
func (c *Client) Fetch(ctx context.Context, key string) (*tracker.Issue, error) {
	path := fmt.Sprintf("/rest/api/2/issue/%s?fields=%s", url.PathEscape(key), issueFields)
	var wire jiraIssue
	if err := c.do(ctx, http.MethodGet, path, nil, &wire); err != nil {
		return nil, err
	}

	issue := &tracker.Issue{
		Key:         wire.Key,
		Project:     wire.Fields.Project.Key,
		IssueType:   wire.Fields.IssueType.Name,
		Summary:     wire.Fields.Summary,
		Description: wire.Fields.Description,
		Status:      wire.Fields.Status.Name,
		Attachments: make([]tracker.Attachment, len(wire.Fields.Attachment)),
	}
	for i, a := range wire.Fields.Attachment {
		issue.Attachments[i] = tracker.Attachment{ID: a.ID, Filename: a.Filename, URL: a.Content}
	}

	comments, err := c.listComments(ctx, key, c.cfg.CommentLimit)
	if err != nil {
		return nil, err
	}
	issue.Comments = chronological(comments)

	return issue, nil
}

// chronological reverses comments — Jira's own newest-first order —
// into oldest-first, the order a human reads a comment thread in, and
// maps each into tracker.Comment. Returns nil for an empty/nil input,
// so ticket.json's optional "comments" field stays absent rather than
// an empty array when comment_limit is 0.
func chronological(comments []jiraComment) []tracker.Comment {
	if len(comments) == 0 {
		return nil
	}
	out := make([]tracker.Comment, len(comments))
	for i, jc := range comments {
		out[len(comments)-1-i] = tracker.Comment{Author: jc.Author.DisplayName, Body: jc.Body, Created: jc.Created}
	}
	return out
}

// CurrentStatus fetches just key's status name — a minimal GET kept
// separate from Fetch so Transition alone stays a single-purpose call
// that does not also pull comments and attachments it has no use for.
// Exported so a caller that only needs the status name (the CLI's
// --simulate preview) can ask for it directly instead of discarding
// the rest of a full Fetch.
func (c *Client) CurrentStatus(ctx context.Context, key string) (string, error) {
	path := fmt.Sprintf("/rest/api/2/issue/%s?fields=status", url.PathEscape(key))
	var wire jiraIssue
	if err := c.do(ctx, http.MethodGet, path, nil, &wire); err != nil {
		return "", err
	}
	return wire.Fields.Status.Name, nil
}
