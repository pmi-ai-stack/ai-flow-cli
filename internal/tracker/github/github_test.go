package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
)

const (
	issuePath    = "/repos/cli/cli/issues/14389"
	commentsPage = "/repos/cli/cli/issues/14389/comments?per_page=100&page=1"
)

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *envelope.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("err = %v, want envelope code %s", err, code)
	}
}

// TestFetchRecordedIssue: a real open issue labelled bug, with eight
// real comments, of which the last three are kept, oldest first.
func TestFetchRecordedIssue(t *testing.T) {
	ft := newFixtureTransport(t).
		on("GET", issuePath, 200, "issue-bug.json").
		on("GET", commentsPage, 200, "comments-page1.json")
	issue, err := newTestClient(t, ft, 3).Fetch(context.Background(), "CLI-14389")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Key != "CLI-14389" || issue.Project != "CLI" {
		t.Errorf("key/project = %s/%s", issue.Key, issue.Project)
	}
	if !strings.HasPrefix(issue.Summary, "`gh release create` does not recognise") || issue.Description == "" {
		t.Errorf("summary = %q, description empty = %v", issue.Summary, issue.Description == "")
	}
	if issue.IssueType != "Bug" {
		t.Errorf("IssueType = %q, want Bug (the bug_types entry matching label \"bug\")", issue.IssueType)
	}
	if issue.Status != "open" {
		t.Errorf("Status = %q, want open (no status label)", issue.Status)
	}
	if len(issue.Attachments) != 0 {
		t.Errorf("Attachments = %v, want none (GitHub has no attachment API)", issue.Attachments)
	}
	if len(issue.Comments) != 3 {
		t.Fatalf("%d comments, want the last 3", len(issue.Comments))
	}
	for i := 1; i < len(issue.Comments); i++ {
		if issue.Comments[i-1].Created > issue.Comments[i].Created {
			t.Errorf("comments not oldest first: %s after %s", issue.Comments[i-1].Created, issue.Comments[i].Created)
		}
	}
	if issue.Comments[0].Author == "" || issue.Comments[0].Body == "" {
		t.Errorf("comment = %+v, want author and body", issue.Comments[0])
	}
}

func TestFetchPullRequestIsNotFound(t *testing.T) {
	ft := newFixtureTransport(t).on("GET", "/repos/cli/cli/issues/14636", 200, "issue-is-pr.json")
	_, err := newTestClient(t, ft, 3).Fetch(context.Background(), "CLI-14636")
	wantCode(t, err, "tracker_not_found")
	if !strings.Contains(err.Error(), "pull request") {
		t.Errorf("err = %v, want it to say pull request", err)
	}
}

func TestUnmappedPrefixIsConfigInvalid(t *testing.T) {
	ft := newFixtureTransport(t)
	_, err := newTestClient(t, ft, 3).Fetch(context.Background(), "PROJ-1")
	wantCode(t, err, "config_invalid")
	if len(ft.calls) != 0 {
		t.Errorf("calls = %v, want none", ft.calls)
	}
}

func TestErrors(t *testing.T) {
	ft := newFixtureTransport(t).
		on("GET", "/user", 401, "error-401.json").
		on("GET", "/repos/cli/cli/issues/99999999", 404, "error-404.json")
	c := newTestClient(t, ft, 3)
	wantCode(t, c.Reachable(context.Background()), "tracker_auth")
	_, err := c.Fetch(context.Background(), "CLI-99999999")
	wantCode(t, err, "tracker_not_found")

	limited := newFixtureTransport(t).onWithHeader("GET", "/user", 403, "", http.Header{"X-Ratelimit-Remaining": []string{"0"}})
	wantCode(t, newTestClient(t, limited, 3).Reachable(context.Background()), "rate_limited")
}

func TestStatusOf(t *testing.T) {
	cfg := testTrackerConfig(3)
	cases := []struct {
		state  string
		labels []string
		want   string
	}{
		{"closed", []string{"in review"}, "closed"},
		{"open", []string{"in progress", "in review"}, "in review"},
		{"open", []string{"bug", "in progress"}, "in progress"},
		{"open", []string{"triage", "bug"}, "triage"},
		{"open", []string{"bug"}, "open"},
		{"open", nil, "open"},
	}
	for _, c := range cases {
		if got := statusOf(cfg, c.state, c.labels); got != c.want {
			t.Errorf("statusOf(%s, %v) = %q, want %q", c.state, c.labels, got, c.want)
		}
	}
}

func TestIssueTypeOf(t *testing.T) {
	cfg := testTrackerConfig(3)
	if got := issueTypeOf(cfg, "Epic", []string{"bug"}); got != "Epic" {
		t.Errorf("an issue type wins: %q", got)
	}
	if got := issueTypeOf(cfg, "", []string{"priority-2", "DEFECT"}); got != "Defect" {
		t.Errorf("case-insensitive bug_types match: %q", got)
	}
	if got := issueTypeOf(cfg, "", []string{"enhancement"}); got != "Issue" {
		t.Errorf("no match: %q", got)
	}
}

// TestTransitionAddsTargetAndRemovesOthers: in progress → in review
// adds the new label, then removes the old one; nothing else is sent.
func TestTransitionAddsTargetAndRemovesOthers(t *testing.T) {
	ft := newFixtureTransport(t).
		on("GET", issuePath, 200, "issue-in-progress.json").
		on("POST", issuePath+"/labels", 200, "").
		on("DELETE", issuePath+"/labels/in%20progress", 200, "")
	if err := newTestClient(t, ft, 3).Transition(context.Background(), "CLI-14389", tracker.StatusClassInReview); err != nil {
		t.Fatal(err)
	}
	if got := ft.bodies["POST "+issuePath+"/labels"]; got != `{"labels":["in review"]}` {
		t.Errorf("POST body = %s", got)
	}
	if want := []string{"GET " + issuePath, "POST " + issuePath + "/labels", "DELETE " + issuePath + "/labels/in%20progress"}; !slices.Equal(ft.calls, want) {
		t.Errorf("calls = %v, want %v", ft.calls, want)
	}
}

func TestTransitionAlreadyThereWritesNothing(t *testing.T) {
	ft := newFixtureTransport(t).on("GET", issuePath, 200, "issue-in-review.json")
	if err := newTestClient(t, ft, 3).Transition(context.Background(), "CLI-14389", tracker.StatusClassInReview); err != nil {
		t.Fatal(err)
	}
	if len(ft.calls) != 1 {
		t.Errorf("calls = %v, want only the GET", ft.calls)
	}
}

// TestTransitionToleratesAnAlreadyRemovedLabel: a label someone removed
// between the read and the delete (404) is the state we wanted.
func TestTransitionToleratesAnAlreadyRemovedLabel(t *testing.T) {
	ft := newFixtureTransport(t).
		on("GET", issuePath, 200, "issue-in-progress.json").
		on("POST", issuePath+"/labels", 200, "").
		on("DELETE", issuePath+"/labels/in%20progress", 404, "error-404.json")
	if err := newTestClient(t, ft, 3).Transition(context.Background(), "CLI-14389", tracker.StatusClassInReview); err != nil {
		t.Errorf("Transition = %v, want nil", err)
	}
}

func TestTransitionTodoIsUsage(t *testing.T) {
	err := newTestClient(t, newFixtureTransport(t), 3).Transition(context.Background(), "CLI-14389", tracker.StatusClassTodo)
	wantCode(t, err, "usage")
}

func TestCommentWithMarkerEditsExisting(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "comments-with-marker.json"))
	if err != nil {
		t.Fatal(err)
	}
	id := extractFourthCommentID(t, data)
	ft := newFixtureTransport(t).
		on("GET", commentsPage, 200, "comments-with-marker.json").
		on("PATCH", "/repos/cli/cli/issues/comments/"+id, 200, "")
	if err := newTestClient(t, ft, 3).Comment(context.Background(), "CLI-14389", "impact", "risk 12"); err != nil {
		t.Fatal(err)
	}
	var sent struct{ Body string }
	if err := json.Unmarshal([]byte(ft.bodies["PATCH /repos/cli/cli/issues/comments/"+id]), &sent); err != nil || sent.Body != "<!-- ai-flow:impact -->\nrisk 12" {
		t.Errorf("PATCH body = %q (%v)", sent.Body, err)
	}
}

func TestCommentWithMarkerPostsWhenAbsent(t *testing.T) {
	ft := newFixtureTransport(t).
		on("GET", commentsPage, 200, "comments-page1.json").
		on("POST", issuePath+"/comments", 201, "")
	if err := newTestClient(t, ft, 3).Comment(context.Background(), "CLI-14389", "impact", "risk 12"); err != nil {
		t.Fatal(err)
	}
	if got := ft.bodies["POST "+issuePath+"/comments"]; !strings.Contains(got, `ai-flow:impact`) {
		t.Errorf("POST body = %s", got)
	}
}

func TestCommentWithoutMarkerPosts(t *testing.T) {
	ft := newFixtureTransport(t).on("POST", issuePath+"/comments", 201, "")
	if err := newTestClient(t, ft, 3).Comment(context.Background(), "CLI-14389", "", "hello"); err != nil {
		t.Fatal(err)
	}
	if got := ft.bodies["POST "+issuePath+"/comments"]; got != `{"body":"hello"}` {
		t.Errorf("POST body = %s", got)
	}
}

func TestStatusesAreLabelsPlusStates(t *testing.T) {
	ft := newFixtureTransport(t).on("GET", "/repos/cli/cli/labels?per_page=100&page=1", 200, "labels-page1.json")
	names, err := newTestClient(t, ft, 3).Statuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "bug") || !slices.Contains(names, "open") || !slices.Contains(names, "closed") {
		t.Errorf("names = %v, want labels plus open and closed", names)
	}
	if !slices.IsSorted(names) || len(names) != 85 {
		t.Errorf("want 83 labels + 2 states, sorted; got %d", len(names))
	}
}

func TestReachable(t *testing.T) {
	ft := newFixtureTransport(t).on("GET", "/user", 200, "")
	if err := newTestClient(t, ft, 3).Reachable(context.Background()); err != nil {
		t.Error(err)
	}
}

func TestDownloadAttachmentsWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	got, err := newTestClient(t, newFixtureTransport(t), 3).DownloadAttachments(context.Background(), &tracker.Issue{}, dir)
	if err != nil || len(got) != 0 {
		t.Errorf("DownloadAttachments = %v, %v; want none", got, err)
	}
}

func extractFourthCommentID(t *testing.T, data []byte) string {
	t.Helper()
	var list []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &list); err != nil || len(list) < 4 {
		t.Fatalf("comments fixture: %v", err)
	}
	return strconv.FormatInt(list[3].ID, 10)
}
