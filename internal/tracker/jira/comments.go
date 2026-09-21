package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// markerPrefix is one line prepended to a marker comment's body: both
// human-readable in the Jira UI and machine-searchable. Jira v2
// comments are plain/wiki-markup text with no field for an opaque
// marker, so a recognizable literal prefix is the mechanism.
const markerPrefix = "[ai-flow:%s]\n"

// markerSearchLimit bounds how many of an issue's most recent comments
// Comment scans for an existing marker — independent of
// config.tracker.comment_limit, which only controls ticket.json's
// display window, not how far back a marker search looks.
const markerSearchLimit = 100

type jiraCommentAuthor struct {
	DisplayName string `json:"displayName"`
}

type jiraComment struct {
	ID      string            `json:"id"`
	Author  jiraCommentAuthor `json:"author"`
	Body    string            `json:"body"`
	Created string            `json:"created"`
}

type jiraCommentList struct {
	Comments []jiraComment `json:"comments"`
}

type jiraCommentBody struct {
	Body string `json:"body"`
}

// listComments fetches key's comments, newest first, capped at limit.
// limit <= 0 skips the call entirely and returns nil, nil — Fetch's
// config.tracker.comment_limit <= 0 branch.
func (c *Client) listComments(ctx context.Context, key string, limit int) ([]jiraComment, error) {
	if limit <= 0 {
		return nil, nil
	}
	path := fmt.Sprintf("/rest/api/2/issue/%s/comment?maxResults=%d&orderBy=-created", url.PathEscape(key), limit)
	var out jiraCommentList
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Comments, nil
}

// Comment implements tracker.Tracker.Comment.
func (c *Client) Comment(ctx context.Context, key, marker, body string) error {
	full := body
	if marker == "" {
		return c.postComment(ctx, key, full)
	}

	prefix := fmt.Sprintf(markerPrefix, marker)
	full = prefix + body

	existing, err := c.listComments(ctx, key, markerSearchLimit)
	if err != nil {
		return err
	}
	for _, ec := range existing {
		if strings.HasPrefix(ec.Body, prefix) {
			return c.editComment(ctx, key, ec.ID, full)
		}
	}
	return c.postComment(ctx, key, full)
}

func (c *Client) postComment(ctx context.Context, key, body string) error {
	path := fmt.Sprintf("/rest/api/2/issue/%s/comment", url.PathEscape(key))
	return c.do(ctx, http.MethodPost, path, jiraCommentBody{Body: body}, nil)
}

func (c *Client) editComment(ctx context.Context, key, commentID, body string) error {
	path := fmt.Sprintf("/rest/api/2/issue/%s/comment/%s", url.PathEscape(key), url.PathEscape(commentID))
	return c.do(ctx, http.MethodPut, path, jiraCommentBody{Body: body}, nil)
}
