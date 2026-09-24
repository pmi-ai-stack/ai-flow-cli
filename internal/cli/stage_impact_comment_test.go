package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// setUpImpactApprovalFixture drives a ticket to `stage complete KEY
// impact` (awaiting_approval, since the fixture's score 40 equals the
// freshly-inited template's own risk_gate 40) — the shared setup every
// test in this file needs before calling `stage approve KEY impact`.
func setUpImpactApprovalFixture(t *testing.T) (home, key string) {
	t.Helper()
	home = t.TempDir()
	key = "PROJ-1"
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeChangedTerraformTarget(t, home, key, true) // impact's own real ShouldRun rule needs a changed target

	runStage(t, home, 0, "start", key, "impact")
	writeFixture(t, home, key, "impact-results", "impact-results.json")
	env := runStage(t, home, 0, "complete", key, "impact")
	if env.Result.State != "awaiting_approval" {
		t.Fatalf("stage complete impact: state = %s, want awaiting_approval (fixture score 40 == risk_gate 40)", env.Result.State)
	}
	return home, key
}

// TestStageApproveImpactPostsMarkerComment proves the marker comment
// is posted with the score, and carries the "[ai-flow:impact]" marker
// prefix jira.Client.Comment's own edit-vs-create search depends on.
func TestStageApproveImpactPostsMarkerComment(t *testing.T) {
	home, key := setUpImpactApprovalFixture(t)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "dev@example.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "token")

	var posted string
	var sawGet, sawPost bool
	useScriptedJira(t, func(r *http.Request) (*http.Response, error) {
		switch r.Method {
		case http.MethodGet:
			sawGet = true
			return jsonResponse(200, `{"comments":[]}`), nil
		case http.MethodPost:
			sawPost = true
			body, _ := io.ReadAll(r.Body)
			posted = string(body)
			return jsonResponse(201, `{}`), nil
		default:
			t.Fatalf("unexpected method %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})

	stdout, _, ec := runRoot(t, "--home", home, "stage", "approve", key, "impact")
	if ec != 0 {
		t.Fatalf("stage approve impact: exitCode = %d\n%s", ec, stdout.String())
	}
	if !sawGet {
		t.Error("never searched existing comments for the marker")
	}
	if !sawPost {
		t.Fatal("never posted a comment")
	}
	if !strings.Contains(posted, "[ai-flow:impact]") {
		t.Errorf("posted body = %q, want the marker prefix", posted)
	}
	if !strings.Contains(posted, "Score: 40/100") {
		t.Errorf("posted body = %q, want the score", posted)
	}
}

// TestStageApproveImpactEditsExistingMarkerComment proves the "edited
// on re-run" half: when a comment already carries the marker prefix,
// jira.Client.Comment edits it (PUT) instead of posting a new one.
func TestStageApproveImpactEditsExistingMarkerComment(t *testing.T) {
	home, key := setUpImpactApprovalFixture(t)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "dev@example.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "token")

	var sawPut bool
	useScriptedJira(t, func(r *http.Request) (*http.Response, error) {
		switch r.Method {
		case http.MethodGet:
			return jsonResponse(200, `{"comments":[{"id":"99","body":"[ai-flow:impact]\nold"}]}`), nil
		case http.MethodPut:
			sawPut = true
			if !strings.HasSuffix(r.URL.Path, "/comment/99") {
				t.Errorf("PUT path = %s, want it to target comment 99", r.URL.Path)
			}
			return jsonResponse(200, `{}`), nil
		case http.MethodPost:
			t.Fatal("posted a new comment instead of editing the existing marker comment")
			return nil, nil
		default:
			t.Fatalf("unexpected method %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})

	stdout, _, ec := runRoot(t, "--home", home, "stage", "approve", key, "impact")
	if ec != 0 {
		t.Fatalf("stage approve impact: exitCode = %d\n%s", ec, stdout.String())
	}
	if !sawPut {
		t.Error("never edited the existing marker comment")
	}
}

// TestStageApproveImpactFailsClosedWithoutTrackerCredentials proves
// the marker-comment side effect fails the whole approve call — no
// half-applied state — when tracker credentials are absent.
func TestStageApproveImpactFailsClosedWithoutTrackerCredentials(t *testing.T) {
	home, key := setUpImpactApprovalFixture(t)

	stdout, _, ec := runRoot(t, "--home", home, "stage", "approve", key, "impact")
	if ec != 5 {
		t.Fatalf("exitCode = %d, want 5 (tracker_auth)\n%s", ec, stdout.String())
	}
}
