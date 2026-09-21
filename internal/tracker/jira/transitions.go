package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

type jiraTransitionTarget struct {
	Name string `json:"name"`
}

type jiraTransition struct {
	ID string               `json:"id"`
	To jiraTransitionTarget `json:"to"`
}

type jiraTransitionList struct {
	Transitions []jiraTransition `json:"transitions"`
}

type jiraTransitionRef struct {
	ID string `json:"id"`
}

type jiraTransitionRequestBody struct {
	Transition jiraTransitionRef `json:"transition"`
}

// TargetStatusName resolves class to the tracker's configured status
// name. todo is never a transition target — the spec's "Ticket side
// effects" table only ever transitions *to* in_progress or in_review;
// todo only appears there as a source class. Exported so a caller that
// needs the name without performing a transition (the CLI's --simulate
// preview) has one place to ask, instead of a second, hand-maintained
// copy of this switch.
func TargetStatusName(cfg config.Tracker, class tracker.StatusClass) (string, bool) {
	switch class {
	case tracker.StatusClassInProgress:
		return cfg.Statuses.InProgress, true
	case tracker.StatusClassInReview:
		return cfg.Statuses.InReview, true
	default:
		return "", false
	}
}

// Transition implements tracker.Tracker.Transition. Idempotent by
// comparing the issue's current status name to the target, not by
// tracking ai-flow's own history — "already in progress" is a no-op
// with no further HTTP calls.
func (c *Client) Transition(ctx context.Context, key string, class tracker.StatusClass) error {
	target, ok := TargetStatusName(c.cfg, class)
	if !ok {
		return &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("jira: %q is not a valid transition target", class),
		}
	}

	current, err := c.CurrentStatus(ctx, key)
	if err != nil {
		return err
	}
	if current == target {
		return nil
	}

	path := fmt.Sprintf("/rest/api/2/issue/%s/transitions", url.PathEscape(key))
	var list jiraTransitionList
	if err := c.do(ctx, http.MethodGet, path, nil, &list); err != nil {
		return err
	}
	for _, t := range list.Transitions {
		if t.To.Name == target {
			return c.do(ctx, http.MethodPost, path, jiraTransitionRequestBody{Transition: jiraTransitionRef{ID: t.ID}}, nil)
		}
	}
	return &envelope.Error{
		Code:    "tool_failed",
		Message: fmt.Sprintf("jira: %s has no transition to %q", key, target),
	}
}
