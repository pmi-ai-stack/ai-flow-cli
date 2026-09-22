package github

import (
	"context"
	"net/http"
)

// githubUser is the minimal shape GET /user returns — this package
// only needs the call to succeed (proving the token authenticates),
// never the identity itself.
type githubUser struct {
	Login string `json:"login"`
}

// Reachable implements host.Host.Reachable: the cheapest authenticated
// GitHub call there is, GET /user. A non-2xx response comes back as
// the same *envelope.Error statusError already maps for every other
// call in this package (host_auth on 401/403, rate_limited on 429).
func (c *Client) Reachable(ctx context.Context) error {
	var wire githubUser
	return c.do(ctx, http.MethodGet, "/user", nil, &wire)
}
