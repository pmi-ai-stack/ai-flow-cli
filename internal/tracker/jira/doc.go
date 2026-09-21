// Package jira implements internal/tracker.Tracker against the Jira
// REST API v2 — plain/wiki-markup text fields, not Atlassian Document
// Format, which is the only respect in which this differs from v3.
// Authentication is HTTP Basic over AI_FLOW_JIRA_EMAIL/AI_FLOW_JIRA_TOKEN,
// read by the caller (internal/cli), never by this package — Client
// takes email and token as constructor parameters, so its own tests
// never need a real environment.
package jira
