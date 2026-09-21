package jira

import (
	"context"
	"net/http"
	"testing"
)

func TestFetch_WithCommentsAndAttachments(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/issue/TEST-1?fields="+issueFields, http.StatusOK, "issue-todo.json").
		on(http.MethodGet, "/rest/api/2/issue/TEST-1/comment?maxResults=5&orderBy=-created", http.StatusOK, "comments-list.json")

	c := New(testTrackerConfig(5), "a@b.com", "tok", ft, nil)

	issue, err := c.Fetch(context.Background(), "TEST-1")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if issue.Key != "TEST-1" || issue.Project != "TEST" || issue.IssueType != "Bug" || issue.Status != "To Do" {
		t.Fatalf("unexpected issue fields: %+v", issue)
	}
	if len(issue.Attachments) != 2 || issue.Attachments[0].Filename != "mock.png" || issue.Attachments[1].Filename != "notes.txt" {
		t.Fatalf("unexpected attachments: %+v", issue.Attachments)
	}
	if len(issue.Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(issue.Comments))
	}
	// chronological: oldest (Alice) first, newest (Bob) last — reversed
	// from the fixture's own newest-first order.
	if issue.Comments[0].Author != "Alice" || issue.Comments[1].Author != "Bob" {
		t.Fatalf("comments not in chronological order: %+v", issue.Comments)
	}
}

func TestFetch_CommentLimitZeroSkipsCommentCall(t *testing.T) {
	// No "comment" fixture registered at all: if the client called the
	// comments endpoint anyway, fixtureTransport would fail the test.
	ft := newFixtureTransport(t).
		on(http.MethodGet, "/rest/api/2/issue/TEST-2?fields="+issueFields, http.StatusOK, "issue-no-attachments.json")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)

	issue, err := c.Fetch(context.Background(), "TEST-2")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if issue.Comments != nil {
		t.Fatalf("expected nil comments, got %+v", issue.Comments)
	}
	if len(issue.Attachments) != 0 {
		t.Fatalf("expected no attachments, got %+v", issue.Attachments)
	}
}
