package jira

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

const statusOnlyPath = "/rest/api/2/issue/TEST-1?fields=status"
const transitionsPath = "/rest/api/2/issue/TEST-1/transitions"

func TestTransition_TodoToInProgressFires(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, statusOnlyPath, http.StatusOK, "status-only-todo.json").
		on(http.MethodGet, transitionsPath, http.StatusOK, "transitions-list.json").
		on(http.MethodPost, transitionsPath, http.StatusNoContent, "")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	if err := c.Transition(context.Background(), "TEST-1", tracker.StatusClassInProgress); err != nil {
		t.Fatalf("Transition: %v", err)
	}
}

func TestTransition_AlreadyInProgressIsNoop(t *testing.T) {
	// No "transitions" fixture registered: if the client called GET or
	// POST /transitions anyway, fixtureTransport would fail the test —
	// this is the idempotent short-circuit's assertion.
	ft := newFixtureTransport(t).
		on(http.MethodGet, statusOnlyPath, http.StatusOK, "status-only-inprogress.json")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	if err := c.Transition(context.Background(), "TEST-1", tracker.StatusClassInProgress); err != nil {
		t.Fatalf("Transition: %v", err)
	}
}

func TestTransition_InProgressToInReviewFires(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, statusOnlyPath, http.StatusOK, "status-only-inprogress.json").
		on(http.MethodGet, transitionsPath, http.StatusOK, "transitions-list.json").
		on(http.MethodPost, transitionsPath, http.StatusNoContent, "")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	if err := c.Transition(context.Background(), "TEST-1", tracker.StatusClassInReview); err != nil {
		t.Fatalf("Transition: %v", err)
	}
}

func TestTransition_AlreadyInReviewIsNoop(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, statusOnlyPath, http.StatusOK, "status-only-inreview.json")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	if err := c.Transition(context.Background(), "TEST-1", tracker.StatusClassInReview); err != nil {
		t.Fatalf("Transition: %v", err)
	}
}

func TestTransition_WorkflowMissingTargetIsToolFailed(t *testing.T) {
	ft := newFixtureTransport(t).
		on(http.MethodGet, statusOnlyPath, http.StatusOK, "status-only-todo.json").
		on(http.MethodGet, transitionsPath, http.StatusOK, "transitions-list-no-match.json")

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	err := c.Transition(context.Background(), "TEST-1", tracker.StatusClassInProgress)

	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "tool_failed" {
		t.Fatalf("expected tool_failed, got %v", err)
	}
}

func TestTransition_TodoClassIsNotAValidTarget(t *testing.T) {
	ft := newFixtureTransport(t) // no fixtures: must fail before any HTTP call

	c := New(testTrackerConfig(0), "a@b.com", "tok", ft, nil)
	err := c.Transition(context.Background(), "TEST-1", tracker.StatusClassTodo)

	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "usage" {
		t.Fatalf("expected usage, got %v", err)
	}
}
