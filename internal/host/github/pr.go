package github

import (
	"context"
	"fmt"
	"net/url"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/host"
)

// githubPR is the subset of GitHub's pull-request JSON EnsurePR needs.
type githubPR struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
}

func (p githubPR) toPullRequest(repo string, reused bool) *host.PullRequest {
	return &host.PullRequest{Repo: repo, Number: p.Number, URL: p.HTMLURL, Draft: p.Draft, Reused: reused}
}

// findOpenPR searches for an open PR with head=owner:branch against
// base, returning nil (not an error) when none exists.
func (c *Client) findOpenPR(ctx context.Context, owner, repo, base, branch string) (*githubPR, error) {
	q := url.Values{
		"head":  {owner + ":" + branch},
		"base":  {base},
		"state": {"open"},
	}
	var found []githubPR
	path := fmt.Sprintf("/repos/%s/%s/pulls?%s", owner, repo, q.Encode())
	if err := c.do(ctx, "GET", path, nil, &found); err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

type createPRRequest struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
}

// EnsurePR implements host.Host: find-or-create, per design decision 12.
func (c *Client) EnsurePR(ctx context.Context, owner, repo, base, branch, title, body string, draft bool) (*host.PullRequest, error) {
	existing, err := c.findOpenPR(ctx, owner, repo, base, branch)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing.toPullRequest(repo, true), nil
	}

	req := createPRRequest{Title: title, Head: branch, Base: base, Body: body, Draft: draft}
	var created githubPR
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	if err := c.do(ctx, "POST", path, req, &created); err != nil {
		return nil, err
	}
	return created.toPullRequest(repo, false), nil
}
