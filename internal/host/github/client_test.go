package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

func TestAuthHeaderFormat(t *testing.T) {
	ft := newFixtureTransport(t).on("GET", "/repos/a/b/pulls?base=main&head=a%3Afeature&state=open", 200, "pulls-empty.json")
	c := New("my-token", ft, nil)

	if c.authHeader != "Bearer my-token" {
		t.Errorf("authHeader = %q, want %q", c.authHeader, "Bearer my-token")
	}
	// Exercise a real call so the header is proven to reach the
	// request, not just the field.
	if _, err := c.findOpenPR(context.Background(), "a", "b", "main", "feature"); err != nil {
		t.Fatalf("findOpenPR: %v", err)
	}
}

func TestStatusCodeMapping(t *testing.T) {
	cases := []struct {
		status   int
		file     string
		wantCode string
	}{
		{401, "error-401.txt", "host_auth"},
		{404, "error-404.txt", "tool_failed"},
		{429, "error-429.txt", "rate_limited"},
	}
	for _, c := range cases {
		t.Run(c.wantCode, func(t *testing.T) {
			ft := newFixtureTransport(t).on("GET", pullsPath("a", "b", "main", "feature"), c.status, c.file)
			client := New("tok", ft, nil)
			client.sleep = func(context.Context, time.Duration) error { return nil }

			_, err := client.findOpenPR(context.Background(), "a", "b", "main", "feature")
			var envErr *envelope.Error
			if !errors.As(err, &envErr) {
				t.Fatalf("err = %v, want *envelope.Error", err)
			}
			if envErr.Code != c.wantCode {
				t.Errorf("Code = %q, want %q", envErr.Code, c.wantCode)
			}
		})
	}
}
