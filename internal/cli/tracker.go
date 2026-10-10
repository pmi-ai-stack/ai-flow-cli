package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/runlog"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
	ghissues "github.com/pmi-ai-stack/ai-flow-cli/internal/tracker/github"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker/jira"
)

// newGitHubIssuesTransport is the GitHub Issues tracker's
// http.RoundTripper source — newJiraTransport's counterpart.
var newGitHubIssuesTransport = func() http.RoundTripper { return http.DefaultTransport }

// trackerCredsSet reports whether the configured provider's credentials
// are present, and names the variables either way, for messages.
func trackerCredsSet(provider string) (bool, string) {
	if provider == config.TrackerGitHub {
		return githubToken() != "", "AI_FLOW_GITHUB_TOKEN (or GITHUB_TOKEN, GH_TOKEN)"
	}
	return os.Getenv("AI_FLOW_JIRA_EMAIL") != "" && os.Getenv("AI_FLOW_JIRA_TOKEN") != "",
		"AI_FLOW_JIRA_EMAIL and AI_FLOW_JIRA_TOKEN"
}

// newTracker is the one place a tracker provider is chosen: Jira or
// GitHub Issues, from config.tracker.provider. It refuses missing
// credentials (tracker_auth) and an insecure base_url (config_invalid)
// before any request carries a credential — nothing forces `config
// validate` to have run first.
func newTracker(ctx context.Context, cfg *config.Config) (tracker.Tracker, error) {
	t := cfg.Tracker
	provider := t.TrackerProvider()
	if provider != config.TrackerJira && provider != config.TrackerGitHub {
		return nil, &envelope.Error{Code: "config_invalid", Message: fmt.Sprintf("tracker.provider: %q is not one of %s, %s", t.Provider, config.TrackerJira, config.TrackerGitHub)}
	}
	if ok, names := trackerCredsSet(provider); !ok {
		return nil, &envelope.Error{Code: "tracker_auth", Message: names + " must be set"}
	}
	if err := config.CheckTrackerBaseURL(t.BaseURL); err != nil {
		return nil, &envelope.Error{Code: "config_invalid", Message: err.Error()}
	}

	timeout := cfg.Defaults.HTTPTimeout()
	if provider == config.TrackerGitHub {
		verbose := runlog.From(ctx).LineWriter("github-issues")
		return ghissues.New(t, githubToken(), newGitHubIssuesTransport(), verbose).WithTimeout(timeout), nil
	}
	verbose := runlog.From(ctx).LineWriter("jira")
	return jira.New(t, os.Getenv("AI_FLOW_JIRA_EMAIL"), os.Getenv("AI_FLOW_JIRA_TOKEN"), newJiraTransport(), verbose).WithTimeout(timeout), nil
}
