package github

import (
	"context"
	"maps"
	"net/http"
	"slices"
)

// Reachable implements tracker.Tracker.Reachable: GET /user, the
// cheapest authenticated call.
func (c *Client) Reachable(ctx context.Context) error {
	var u ghUser
	return c.do(ctx, http.MethodGet, "/user", nil, &u)
}

// Statuses implements tracker.Tracker.Statuses: every label of every
// configured repository, plus the "open" and "closed" states — what
// doctor checks config.tracker.statuses' names against. Sorted and
// deduped.
func (c *Client) Statuses(ctx context.Context) ([]string, error) {
	names := map[string]bool{stateOpen: true, stateClosed: true}
	repos := slices.Sorted(maps.Values(c.cfg.Repositories))
	for _, repo := range slices.Compact(repos) {
		labels, err := pages[ghLabel](ctx, c, "/repos/"+repo+"/labels")
		if err != nil {
			return nil, err
		}
		for _, l := range labels {
			names[l.Name] = true
		}
	}
	return slices.Sorted(maps.Keys(names)), nil
}
