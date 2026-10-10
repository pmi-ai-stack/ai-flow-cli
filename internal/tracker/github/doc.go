// Package github implements internal/tracker.Tracker against GitHub
// Issues over the REST API — the second tracker provider, selected by
// config.tracker.provider "github".
//
// Ticket keys keep ai-flow's one shape, <PREFIX>-<number>:
// config.tracker.repositories maps the prefix to the owner/repo whose
// issue number it is ("CLI" → "cli/cli": CLI-42 is cli/cli#42), so
// ticket directories, branch names and routing need nothing new.
//
// GitHub issues are only open or closed, so the configured status
// names are labels (status.go): ai-flow sets the in_progress and
// in_review labels, and an open issue with no configured status label
// reports "open", which the github template lists as todo. GitHub has
// no attachment API; files pasted into an issue stay links in its body.
//
// Authentication is a bearer token (AI_FLOW_GITHUB_TOKEN, else
// GITHUB_TOKEN, else GH_TOKEN), read by the caller — never by this
// package, so its tests need no real environment.
package github
