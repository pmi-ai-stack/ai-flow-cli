package gitops

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestParseOwnerRepo(t *testing.T) {
	cases := []struct {
		url             string
		wantOwner, repo string
	}{
		{"git@github.com:acme/billing-api.git", "acme", "billing-api"},
		{"https://github.com/acme/billing-api.git", "acme", "billing-api"},
		{"https://github.com/acme/billing-api", "acme", "billing-api"},
		{"ssh://git@github.com/acme/billing-api.git", "acme", "billing-api"},
		{"https://ghe.acme.internal/acme/billing-api.git", "acme", "billing-api"},
	}
	for _, c := range cases {
		owner, repo, err := parseOwnerRepo(c.url)
		if err != nil {
			t.Errorf("parseOwnerRepo(%q): %v", c.url, err)
			continue
		}
		if owner != c.wantOwner || repo != c.repo {
			t.Errorf("parseOwnerRepo(%q) = (%q, %q), want (%q, %q)", c.url, owner, repo, c.wantOwner, c.repo)
		}
	}
}

func TestRemoteOwnerRepo(t *testing.T) {
	fe := exectest.New(t).On("git", []string{"remote", "get-url", "origin"}, exec.Output{Stdout: "git@github.com:acme/billing-api.git\n"})
	g := New(fe, 0)

	owner, repo, err := g.RemoteOwnerRepo(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("RemoteOwnerRepo: %v", err)
	}
	if owner != "acme" || repo != "billing-api" {
		t.Errorf("got (%q, %q), want (acme, billing-api)", owner, repo)
	}
}
