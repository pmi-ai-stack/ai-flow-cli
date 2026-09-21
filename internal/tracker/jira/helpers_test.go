package jira

import "github.com/pmi-ai-stack/ai-flow-cli/internal/config"

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
