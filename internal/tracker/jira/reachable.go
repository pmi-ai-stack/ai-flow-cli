package jira

import (
	"context"
	"net/http"
)

// jiraMyself is the minimal shape /rest/api/2/myself returns — this
// package only needs the call to succeed (proving the credentials
// authenticate), never the identity itself.
type jiraMyself struct {
	AccountID string `json:"accountId"`
}

// Reachable implements tracker.Tracker.Reachable: the cheapest
// authenticated Jira call there is, /rest/api/2/myself. A non-2xx
// response comes back as the same *envelope.Error statusError already
// maps for every other call in this package (tracker_auth on 401/403,
// rate_limited on 429).
func (c *Client) Reachable(ctx context.Context) error {
	var wire jiraMyself
	return c.do(ctx, http.MethodGet, "/rest/api/2/myself", nil, &wire)
}

// Statuses implements tracker.Tracker.Statuses: every status name
// configured across the instance's workflow(s), independent of any one
// issue's own transitions — doctor has no ticket key to scope a
// per-issue call to.
func (c *Client) Statuses(ctx context.Context) ([]string, error) {
	var wire []jiraStatus
	if err := c.do(ctx, http.MethodGet, "/rest/api/2/status", nil, &wire); err != nil {
		return nil, err
	}
	names := make([]string, len(wire))
	for i, s := range wire {
		names[i] = s.Name
	}
	return names, nil
}
