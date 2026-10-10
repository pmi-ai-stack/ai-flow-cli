package github

import (
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// Statuses an issue reports that are GitHub states, not labels.
const (
	stateOpen   = "open"
	stateClosed = "closed"
)

// statusOf is an issue's status name, first match wins: closed; the
// in_review label; the in_progress label; the first configured todo
// label present; otherwise "open" — which a github configuration lists
// under statuses.todo, so `ticket fetch` moves an untouched issue to
// in_progress exactly as it moves a Jira "To Do".
func statusOf(cfg config.Tracker, state string, labels []string) string {
	if state == stateClosed {
		return stateClosed
	}
	has := func(name string) bool {
		for _, l := range labels {
			if name != "" && l == name {
				return true
			}
		}
		return false
	}
	switch {
	case has(cfg.Statuses.InReview):
		return cfg.Statuses.InReview
	case has(cfg.Statuses.InProgress):
		return cfg.Statuses.InProgress
	}
	for _, todo := range cfg.Statuses.Todo {
		if has(todo) {
			return todo
		}
	}
	return stateOpen
}

// statusLabels is every configured status name that is a label (the
// "open" pseudo-status is a state, never a label).
func statusLabels(cfg config.Tracker) []string {
	var out []string
	for _, n := range append(append([]string{}, cfg.Statuses.Todo...), cfg.Statuses.InProgress, cfg.Statuses.InReview) {
		if n != "" && n != stateOpen && n != stateClosed {
			out = append(out, n)
		}
	}
	return out
}

// issueTypeOf is the issue's type for branch naming: GitHub's own issue
// type when the organisation uses them; otherwise the bug_types entry
// (in configured order, case-insensitively) matching one of its
// labels, so a "bug" label yields bugfix/<KEY>; otherwise "Issue".
func issueTypeOf(cfg config.Tracker, typeName string, labels []string) string {
	if typeName != "" {
		return typeName
	}
	for _, bt := range cfg.BugTypes {
		for _, l := range labels {
			if strings.EqualFold(bt, l) {
				return bt
			}
		}
	}
	return "Issue"
}
