package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker/jira"
)

func newTicketCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ticket",
		Short: "Fetch, update and show a ticket from the configured tracker",
	}
	cmd.AddCommand(newTicketFetchCmd(cio))
	cmd.AddCommand(newTicketUpdateCmd(cio))
	cmd.AddCommand(newTicketShowCmd(cio))
	return cmd
}

// ticketComment mirrors one item of ticket.schema.json's comments[].
type ticketComment struct {
	Author  string `json:"author"`
	Body    string `json:"body"`
	Created string `json:"created"`
}

// ticketArtifact mirrors ticket.schema.json exactly — additionalProperties
// is false there, so this struct's json tags are the schema's full
// property list, no more, no less. handoff.WriteArtifact validates
// every value of this type against that schema before it is ever
// written, so a mismatch here fails loudly (invalid_result) rather
// than silently drifting.
type ticketArtifact struct {
	Key         string          `json:"key"`
	Project     string          `json:"project,omitempty"`
	IssueType   string          `json:"issue_type,omitempty"`
	Summary     string          `json:"summary"`
	Description string          `json:"description,omitempty"`
	Status      string          `json:"status"`
	BranchName  string          `json:"branch_name"`
	Comments    []ticketComment `json:"comments,omitempty"`
	Attachments []string        `json:"attachments,omitempty"`
}

func toTicketComments(cs []tracker.Comment) []ticketComment {
	if len(cs) == 0 {
		return nil
	}
	out := make([]ticketComment, len(cs))
	for i, c := range cs {
		out[i] = ticketComment{Author: c.Author, Body: c.Body, Created: c.Created}
	}
	return out
}

// branchName implements ticket.schema.json's own documented rule:
// bugfix/<KEY> when issueType is one of bugTypes, else feature/<KEY>.
func branchName(key, issueType string, bugTypes []string) string {
	if slices.Contains(bugTypes, issueType) {
		return "bugfix/" + key
	}
	return "feature/" + key
}

// inTodoClass reports whether status is one of the tracker's
// configured todo-class names — the trigger for fetch's in_progress
// transition (the spec's "Ticket side effects" table).
func inTodoClass(status string, todo []string) bool {
	return slices.Contains(todo, status)
}

// newJiraTransport is buildTracker's http.RoundTripper source —
// mirrors pr.go's newGitHubTransport exactly: a package-level var so a
// test can substitute a fixture transport instead of ever making a
// real HTTP call (doctor's own cli-level test is the first to need
// this for Jira; pr_test.go already relies on the GitHub equivalent).
var newJiraTransport = func() http.RoundTripper { return http.DefaultTransport }

// buildTracker reads AI_FLOW_JIRA_EMAIL/AI_FLOW_JIRA_TOKEN and builds a
// jira.Client. Read here, never inside the jira package itself
// (internal/tracker/jira's own doc comment); missing either variable
// is tracker_auth before any HTTP call is attempted.
func buildTracker(cmd *cobra.Command, f Flags) (*jira.Client, *config.Config, error) {
	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return nil, nil, err
	}

	email := os.Getenv("AI_FLOW_JIRA_EMAIL")
	token := os.Getenv("AI_FLOW_JIRA_TOKEN")
	if email == "" || token == "" {
		return nil, nil, &envelope.Error{
			Code:    "tracker_auth",
			Message: "AI_FLOW_JIRA_EMAIL and AI_FLOW_JIRA_TOKEN must both be set",
		}
	}

	var verbose io.Writer
	if f.Verbose {
		verbose = redact.New(os.Environ()).Writer(cmd.ErrOrStderr())
	}

	return jira.New(cfg.Tracker, email, token, newJiraTransport(), verbose), cfg, nil
}

// redactErr passes a *envelope.Error's Message/Details through
// redact.New(os.Environ()) before it reaches the envelope — every
// tracker call in this file routes its returned error through this
// (07-determinism.md: secrets are redacted everywhere, including error
// messages, in case a response body ever echoes one back). Anything
// that is not a *envelope.Error, and nil, pass through unchanged.
func redactErr(err error) error {
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		return err
	}
	r := redact.New(os.Environ())
	out := &envelope.Error{Code: envErr.Code, Message: r.String(envErr.Message)}
	for _, d := range envErr.Details {
		out.Details = append(out.Details, r.String(d))
	}
	return out
}

var imageExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"}

func isImageFilename(name string) bool {
	return slices.Contains(imageExtensions, strings.ToLower(filepath.Ext(name)))
}

// writeImagesMD writes ticketDir/images.md: every filename in
// filenames (the attachments just downloaded) whose extension is an
// image type, rendered as a Markdown image link, sorted for byte
// stability; a fixed placeholder line when none qualify.
func writeImagesMD(ticketDir string, filenames []string) (string, error) {
	var images []string
	for _, f := range filenames {
		if isImageFilename(f) {
			images = append(images, f)
		}
	}
	sort.Strings(images)

	var b strings.Builder
	b.WriteString("# Images\n\n")
	if len(images) == 0 {
		b.WriteString("(no image attachments)\n")
	} else {
		for _, f := range images {
			fmt.Fprintf(&b, "![%s](attachments/%s)\n", f, f)
		}
	}
	return handoff.WriteFile(ticketDir, "images.md", []byte(b.String()))
}

// driveFetchStage drives the fetch stage from pending to pass in one
// transaction (StageTable's Command: "ticket fetch" — no skill ever
// starts this stage). On a second run, when the stage is already
// pass/skipped/approved, this does nothing: the tracker call and
// artifact writes above still ran, so the command as a whole stays
// idempotent without attempting a transition handoff.Start would
// refuse (it only accepts pending/awaiting_approval as an origin).
func driveFetchStage(f Flags, key string) error {
	st, err := handoff.LoadStatus(f.Home, key)
	if err != nil {
		return err
	}
	from := st.Entry(handoff.StageFetch).State

	switch from {
	case handoff.StatePending:
		if err := handoff.Start(st, handoff.StageFetch, handoff.ShouldRunInput{}); err != nil {
			return err
		}
	case handoff.StateRunning:
		// A prior crash mid-fetch; fall through to Complete below.
	default:
		return nil
	}

	if err := handoff.Complete(st, handoff.StageFetch, false, handoff.GateInputs{}); err != nil {
		return err
	}
	to := st.Entry(handoff.StageFetch).State

	return finalizeTransition(f.Home, key, handoff.StageFetch, st, "ticket fetch", from, to, "")
}

// ticketFetchSimulateResult is `ticket fetch --simulate`'s result: the
// read already happened, but every write is described, not performed.
type ticketFetchSimulateResult struct {
	Key               string  `json:"key"`
	Status            string  `json:"status"`
	WouldTransitionTo *string `json:"would_transition_to"`
}

func newTicketFetchCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "fetch KEY",
		Short: "Fetch a ticket from the tracker and complete the fetch stage",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ticket fetch", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			trk, cfg, err := buildTracker(cmd, f)
			if err != nil {
				return Outcome{}, err
			}

			issue, err := trk.Fetch(cmd.Context(), key)
			if err != nil {
				return Outcome{}, redactErr(err)
			}

			todoClass := inTodoClass(issue.Status, cfg.Tracker.Statuses.Todo)

			if f.Simulate {
				var would *string
				if todoClass {
					target := cfg.Tracker.Statuses.InProgress
					would = &target
				}
				return Outcome{Result: ticketFetchSimulateResult{Key: key, Status: issue.Status, WouldTransitionTo: would}}, nil
			}

			finalStatus := issue.Status
			if todoClass {
				if err := trk.Transition(cmd.Context(), key, tracker.StatusClassInProgress); err != nil {
					return Outcome{}, redactErr(err)
				}
				finalStatus = cfg.Tracker.Statuses.InProgress
			}

			ticketDir := handoff.TicketDir(f.Home, key)
			attachmentsDir := filepath.Join(ticketDir, "attachments")
			written, err := trk.DownloadAttachments(cmd.Context(), issue, attachmentsDir)
			if err != nil {
				return Outcome{}, redactErr(err)
			}

			artifact := ticketArtifact{
				Key:         issue.Key,
				Project:     issue.Project,
				IssueType:   issue.IssueType,
				Summary:     issue.Summary,
				Description: issue.Description,
				Status:      finalStatus,
				BranchName:  branchName(key, issue.IssueType, cfg.Tracker.BugTypes),
				Comments:    toTicketComments(issue.Comments),
				Attachments: written,
			}
			if _, err := handoff.WriteArtifact(ticketDir, "ticket.json", "ticket", artifact); err != nil {
				return Outcome{}, err
			}
			if _, err := writeImagesMD(ticketDir, written); err != nil {
				return Outcome{}, err
			}

			if err := driveFetchStage(f, key); err != nil {
				return Outcome{}, err
			}

			return Outcome{Result: artifact}, nil
		}),
	}
}

// ticketUpdateResult is `ticket update`'s non-simulate result.
type ticketUpdateResult struct {
	Key            string `json:"key"`
	TransitionedTo string `json:"transitioned_to,omitempty"`
	CommentPosted  bool   `json:"comment_posted"`
}

// ticketUpdateSimulateResult is `ticket update --simulate`'s result.
type ticketUpdateSimulateResult struct {
	Key               string  `json:"key"`
	WouldTransitionTo *string `json:"would_transition_to"`
	WouldPostComment  *string `json:"would_post_comment"`
}

func newTicketUpdateCmd(cio cmdIO) *cobra.Command {
	var statusFlag, commentFlag, markerFlag string
	cmd := &cobra.Command{
		Use:   "update KEY",
		Short: "Transition a ticket and/or post or edit a tracker comment",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ticket update", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			if statusFlag == "" && commentFlag == "" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "at least one of --status or --comment is required"}
			}

			var class tracker.StatusClass
			if statusFlag != "" {
				switch statusFlag {
				case "in_progress":
					class = tracker.StatusClassInProgress
				case "in_review":
					class = tracker.StatusClassInReview
				default:
					return Outcome{}, &envelope.Error{
						Code:    "usage",
						Message: fmt.Sprintf("--status %q is not one of: in_progress, in_review", statusFlag),
					}
				}
			}

			f := FlagsFrom(cmd.Context())
			trk, cfg, err := buildTracker(cmd, f)
			if err != nil {
				return Outcome{}, err
			}

			if f.Simulate {
				var wouldTransition *string
				if statusFlag != "" {
					current, err := trk.CurrentStatus(cmd.Context(), key)
					if err != nil {
						return Outcome{}, redactErr(err)
					}
					target, _ := jira.TargetStatusName(cfg.Tracker, class)
					if current != target {
						wouldTransition = &target
					}
				}
				var wouldComment *string
				if commentFlag != "" {
					wouldComment = &commentFlag
				}
				return Outcome{Result: ticketUpdateSimulateResult{Key: key, WouldTransitionTo: wouldTransition, WouldPostComment: wouldComment}}, nil
			}

			result := ticketUpdateResult{Key: key}
			if statusFlag != "" {
				if err := trk.Transition(cmd.Context(), key, class); err != nil {
					return Outcome{}, redactErr(err)
				}
				result.TransitionedTo, _ = jira.TargetStatusName(cfg.Tracker, class)
			}
			if commentFlag != "" {
				if err := trk.Comment(cmd.Context(), key, markerFlag, commentFlag); err != nil {
					return Outcome{}, redactErr(err)
				}
				result.CommentPosted = true
			}
			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().StringVar(&statusFlag, "status", "", "transition target class: in_progress or in_review")
	cmd.Flags().StringVar(&commentFlag, "comment", "", "comment body to post or edit")
	cmd.Flags().StringVar(&markerFlag, "marker", "", "marker name: edit the existing comment carrying it instead of posting a new one")
	return cmd
}

func newTicketShowCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "show KEY",
		Short: "Print a ticket's current tracker state, without writing anything",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ticket show", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			trk, cfg, err := buildTracker(cmd, f)
			if err != nil {
				return Outcome{}, err
			}

			issue, err := trk.Fetch(cmd.Context(), key)
			if err != nil {
				return Outcome{}, redactErr(err)
			}

			attachments := make([]string, len(issue.Attachments))
			for i, a := range issue.Attachments {
				attachments[i] = a.Filename
			}

			result := ticketArtifact{
				Key:         issue.Key,
				Project:     issue.Project,
				IssueType:   issue.IssueType,
				Summary:     issue.Summary,
				Description: issue.Description,
				Status:      issue.Status,
				BranchName:  branchName(key, issue.IssueType, cfg.Tracker.BugTypes),
				Comments:    toTicketComments(issue.Comments),
				Attachments: attachments,
			}
			return Outcome{Result: result}, nil
		}),
	}
}
