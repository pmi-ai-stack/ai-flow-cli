package jira

import (
	"context"
	"net/http"
	"testing"
)

func TestComment_NoMarkerAlwaysPosts(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodPost, "/rest/api/2/issue/TEST-1/comment", http.StatusCreated, "")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	if err := c.Comment(context.Background(), "TEST-1", "", "plain comment"); err != nil {
		t.Fatalf("Comment: %v", err)
	}
}

func TestComment_MarkerAbsentCreates(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/issue/TEST-1/comment?maxResults=100&orderBy=-created", http.StatusOK, "comments-empty.json").
		on(http.MethodPost, "/rest/api/2/issue/TEST-1/comment", http.StatusCreated, "")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	if err := c.Comment(context.Background(), "TEST-1", "ship", "PR links here"); err != nil {
		t.Fatalf("Comment: %v", err)
	}
}

func TestComment_MarkerPresentEdits(t *testing.T) {
	// No POST fixture registered: if the client created a new comment
	// instead of editing, fixtureTransport would fail the test — the
	// marker-edit-idempotency assertion.
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/issue/TEST-1/comment?maxResults=100&orderBy=-created", http.StatusOK, "comments-with-marker.json").
		on(http.MethodPut, "/rest/api/2/issue/TEST-1/comment/300", http.StatusOK, "")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	if err := c.Comment(context.Background(), "TEST-1", "ship", "Updated ship comment body"); err != nil {
		t.Fatalf("Comment: %v", err)
	}
}
