package jira

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

func TestReachable_OK(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/myself", http.StatusOK, "myself.json")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	if err := c.Reachable(context.Background()); err != nil {
		t.Fatalf("Reachable: %v", err)
	}
}

func TestReachable_AuthFailure(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/myself", http.StatusUnauthorized, "error-401.txt")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	err := c.Reachable(context.Background())
	if err == nil {
		t.Fatal("Reachable: err = nil, want tracker_auth")
	}
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "tracker_auth" {
		t.Errorf("Reachable: err = %v, want error.code tracker_auth", err)
	}
}

func TestStatuses_OK(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/status", http.StatusOK, "statuses-list.json")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	got, err := c.Statuses(context.Background())
	if err != nil {
		t.Fatalf("Statuses: %v", err)
	}
	want := []string{"To Do", "Backlog", "In Progress", "In Review", "Done"}
	if !slices.Equal(got, want) {
		t.Errorf("Statuses = %v, want %v", got, want)
	}
}

func TestStatuses_RateLimited(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/status", http.StatusTooManyRequests, "error-429.txt")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	recordSleeps(c)
	_, err := c.Statuses(context.Background())
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "rate_limited" {
		t.Errorf("Statuses: err = %v, want error.code rate_limited", err)
	}
}
