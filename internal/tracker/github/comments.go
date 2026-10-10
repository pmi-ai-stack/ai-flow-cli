package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// markerPrefix opens a comment ai-flow owns: an HTML comment, invisible
// in GitHub's rendering, on the comment's first line.
const markerPrefix = "<!-- ai-flow:%s -->\n"

type ghUser struct {
	Login string `json:"login"`
}

type ghComment struct {
	ID        int64  `json:"id"`
	User      ghUser `json:"user"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// listComments returns the issue's comments, oldest first (the API's
// own order), up to maxPages pages.
func (c *Client) listComments(ctx context.Context, ref issueRef) ([]ghComment, error) {
	return pages[ghComment](ctx, c, ref.path("/comments"))
}

// Comment implements tracker.Tracker.Comment: with a marker, the
// comment already carrying it is edited in place; otherwise a new one
// is posted. A POST is never retried after a gateway error (httpretry),
// so a comment is not duplicated by a retry either.
func (c *Client) Comment(ctx context.Context, key, marker, body string) error {
	ref, err := c.resolve(key)
	if err != nil {
		return err
	}
	if marker == "" {
		return c.do(ctx, http.MethodPost, ref.path("/comments"), map[string]string{"body": body}, nil)
	}
	prefix := fmt.Sprintf(markerPrefix, marker)
	existing, err := c.listComments(ctx, ref)
	if err != nil {
		return err
	}
	for _, ec := range existing {
		if strings.HasPrefix(ec.Body, prefix) {
			path := "/repos/" + ref.repo + "/issues/comments/" + strconv.FormatInt(ec.ID, 10)
			return c.do(ctx, http.MethodPatch, path, map[string]string{"body": prefix + body}, nil)
		}
	}
	return c.do(ctx, http.MethodPost, ref.path("/comments"), map[string]string{"body": prefix + body}, nil)
}

// isNotFound reports whether err is this package's tracker_not_found.
func isNotFound(err error) bool {
	var e *envelope.Error
	return errors.As(err, &e) && e.Code == "tracker_not_found"
}
