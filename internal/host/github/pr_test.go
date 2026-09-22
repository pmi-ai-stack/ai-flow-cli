package github

import (
	"context"
	"fmt"
	"net/url"
	"testing"
)

func pullsPath(owner, repo, base, branch string) string {
	q := url.Values{"head": {owner + ":" + branch}, "base": {base}, "state": {"open"}}
	return fmt.Sprintf("/repos/%s/%s/pulls?%s", owner, repo, q.Encode())
}

func TestEnsurePRReusesExistingOpenPR(t *testing.T) {
	ft := newFixtureTransport(t).
		on("GET", pullsPath("acme", "billing-api", "main", "feature/PROJ-1"), 200, "pulls-existing.json")
	c := New("tok", ft, nil)

	pr, err := c.EnsurePR(context.Background(), "acme", "billing-api", "main", "feature/PROJ-1", "PROJ-1: fix", "body", false)
	if err != nil {
		t.Fatalf("EnsurePR: %v", err)
	}
	if !pr.Reused {
		t.Error("Reused = false, want true")
	}
	if pr.Number != 42 || pr.URL != "https://github.com/acme/billing-api/pull/42" {
		t.Errorf("pr = %+v, want number 42", pr)
	}
	// No POST fixture is registered — a POST here would fail the test.
}

func TestEnsurePRCreatesWhenNoneOpen(t *testing.T) {
	ft := newFixtureTransport(t).
		on("GET", pullsPath("acme", "billing-api", "main", "feature/PROJ-1"), 200, "pulls-empty.json").
		on("POST", "/repos/acme/billing-api/pulls", 201, "pull-created.json")
	c := New("tok", ft, nil)

	pr, err := c.EnsurePR(context.Background(), "acme", "billing-api", "main", "feature/PROJ-1", "PROJ-1: fix", "body", true)
	if err != nil {
		t.Fatalf("EnsurePR: %v", err)
	}
	if pr.Reused {
		t.Error("Reused = true, want false")
	}
	if pr.Number != 43 || !pr.Draft {
		t.Errorf("pr = %+v, want number 43, draft true", pr)
	}
}

func TestEnsurePRPropagatesAuthError(t *testing.T) {
	ft := newFixtureTransport(t).
		on("GET", pullsPath("acme", "billing-api", "main", "feature/PROJ-1"), 401, "error-401.txt")
	c := New("bad-tok", ft, nil)

	_, err := c.EnsurePR(context.Background(), "acme", "billing-api", "main", "feature/PROJ-1", "t", "b", false)
	if err == nil {
		t.Fatal("expected an error")
	}
}
