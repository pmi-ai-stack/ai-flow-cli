package github

import (
	"context"
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

func TestReachable_OK(t *testing.T) {
	ft := newFixtureTransport(t).on("GET", "/user", 200, "user.json")
	c := New("tok", ft, nil)

	if err := c.Reachable(context.Background()); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
}

func TestReachable_AuthFailure(t *testing.T) {
	ft := newFixtureTransport(t).on("GET", "/user", 401, "error-401.txt")
	c := New("tok", ft, nil)

	err := c.Reachable(context.Background())
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "host_auth" {
		t.Errorf("Reachable: err = %v, want error.code host_auth", err)
	}
}
