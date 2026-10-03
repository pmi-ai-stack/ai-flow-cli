package jira

import (
	"context"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// testTrackerConfig is the config.tracker shape every test in this
// package builds a Client against — a trimmed version of the spec's
// own config.json example.
func testTrackerConfig(commentLimit int) config.Tracker {
	return config.Tracker{
		Provider: "jira",
		BaseURL:  "https://example.atlassian.net",
		Statuses: config.TrackerStatuses{
			Todo:       []string{"To Do", "Backlog"},
			InProgress: "In Progress",
			InReview:   "In Review",
		},
		BugTypes:     []string{"Bug", "Defect"},
		CommentLimit: commentLimit,
	}
}

// recordSleeps replaces c's retry backoff with a recorder, so a test
// exercising a retried status costs no wall time.
func recordSleeps(c *Client) *[]time.Duration {
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	return &slept
}
