package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// writeTicketTestConfig writes a minimal but complete config.json to
// home, with tracker.base_url pointing at a local fake Jira server —
// config.Load decodes every field DisallowUnknownFields, so every
// section from the spec's own example is present, even though only
// .tracker matters to any command exercised in this file.
func writeTicketTestConfig(t *testing.T, home, baseURL string) {
	t.Helper()
	cfg := fmt.Sprintf(`{
  "schema_version": 1,
  "tracker": {
    "provider": "jira",
    "base_url": %q,
    "statuses": { "todo": ["To Do", "Backlog"], "in_progress": "In Progress", "in_review": "In Review" },
    "bug_types": ["Bug", "Defect"],
    "comment_limit": 5
  },
  "host": { "provider": "github", "default_base_branch": "main", "draft_prs": false },
  "routing": { "project_key_map": {}, "keyword_to_repo": {}, "teams": {} },
  "defaults": { "worktree": false, "viewport": "1440x900", "mutation_threshold": 80, "mutation_max_rounds": 3, "mutation_timeout_minutes": 20, "test_timeout_minutes": 20, "guard_idle_hours": 8 },
  "standards": { "sources": [], "guidance_budget_tokens": 6000 },
  "ops": { "environments": {}, "target_map": {}, "default_environment": "", "risk_gate": 40, "risk": { "action_weight": {}, "kind_weight": {}, "tier_multiplier": {}, "drift_penalty": 5, "offline_penalty": 10 }, "protected_kinds": [], "e2e": { "enabled": false, "sandbox_environment": "" }, "tool_timeout_minutes": 30 }
}`, baseURL)
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

// fakeJira is a minimal, in-memory Jira REST API v2 double — a local
// httptest.Server, never a real Jira instance (07-determinism.md: tests
// run offline). It tracks call counts so tests can assert a write did
// or did not happen without inspecting HTTP traffic directly.
type fakeJira struct {
	mu              sync.Mutex
	status          string
	comments        []fakeComment
	commentPosts    int
	commentPuts     int
	transitionCalls int
}

type fakeComment struct {
	id, author, body string
}

// newFakeJiraServer starts the double with issue TEST-1 at
// initialStatus, one attachment (shot.png) and no comments.
func newFakeJiraServer(t *testing.T, initialStatus string) (*httptest.Server, *fakeJira) {
	t.Helper()
	fj := &fakeJira{status: initialStatus}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rest/api/2/issue/{key}", func(w http.ResponseWriter, r *http.Request) {
		fj.mu.Lock()
		status := fj.status
		fj.mu.Unlock()
		if r.URL.Query().Get("fields") == "status" {
			fmt.Fprintf(w, `{"key":%q,"fields":{"status":{"name":%q}}}`, r.PathValue("key"), status)
			return
		}
		fmt.Fprintf(w, `{"key":%q,"fields":{"summary":"Summary","description":"Desc","status":{"name":%q},`+
			`"issuetype":{"name":"Bug"},"project":{"key":"TEST"},`+
			`"attachment":[{"id":"1","filename":"shot.png","content":"http://%s/attachments/shot.png"}]}}`,
			r.PathValue("key"), status, r.Host)
	})

	mux.HandleFunc("GET /attachments/shot.png", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fake-png-bytes")
	})

	mux.HandleFunc("GET /rest/api/2/issue/{key}/comment", func(w http.ResponseWriter, r *http.Request) {
		fj.mu.Lock()
		defer fj.mu.Unlock()
		fmt.Fprint(w, `{"comments":[`)
		for i, c := range fj.comments {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"id":%q,"author":{"displayName":%q},"body":%q,"created":"2026-09-21T00:00:00.000+0000"}`, c.id, c.author, c.body)
		}
		fmt.Fprint(w, `]}`)
	})

	mux.HandleFunc("POST /rest/api/2/issue/{key}/comment", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fj.mu.Lock()
		fj.commentPosts++
		id := fmt.Sprintf("%d", len(fj.comments)+1)
		fj.comments = append([]fakeComment{{id: id, author: "ai-flow", body: body.Body}}, fj.comments...)
		fj.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("PUT /rest/api/2/issue/{key}/comment/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := r.PathValue("id")
		fj.mu.Lock()
		fj.commentPuts++
		for i, c := range fj.comments {
			if c.id == id {
				fj.comments[i].body = body.Body
			}
		}
		fj.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /rest/api/2/issue/{key}/transitions", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"transitions":[{"id":"11","to":{"name":"In Progress"}},{"id":"21","to":{"name":"In Review"}}]}`)
	})

	mux.HandleFunc("POST /rest/api/2/issue/{key}/transitions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Transition struct {
				ID string `json:"id"`
			} `json:"transition"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fj.mu.Lock()
		fj.transitionCalls++
		switch body.Transition.ID {
		case "11":
			fj.status = "In Progress"
		case "21":
			fj.status = "In Review"
		}
		fj.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, fj
}

func TestTicketFetch_WritesArtifactsAndCompletesStage(t *testing.T) {
	home := t.TempDir()
	srv, fj := newFakeJiraServer(t, "To Do")
	writeTicketTestConfig(t, home, srv.URL)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	stdout, _, exitCode := runRoot(t, "ticket", "fetch", "TEST-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(home, "TEST-1", "ticket.json"))
	if err != nil {
		t.Fatalf("ticket.json not written: %v", err)
	}
	var artifact struct {
		Status      string   `json:"status"`
		BranchName  string   `json:"branch_name"`
		Attachments []string `json:"attachments"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("ticket.json invalid JSON: %v", err)
	}
	if artifact.Status != "In Progress" {
		t.Errorf("status = %q, want In Progress (fetch should transition a todo-class ticket)", artifact.Status)
	}
	if artifact.BranchName != "bugfix/TEST-1" {
		t.Errorf("branch_name = %q, want bugfix/TEST-1", artifact.BranchName)
	}
	if len(artifact.Attachments) != 1 || artifact.Attachments[0] != "shot.png" {
		t.Errorf("attachments = %v, want [shot.png]", artifact.Attachments)
	}

	if _, err := os.Stat(filepath.Join(home, "TEST-1", "attachments", "shot.png")); err != nil {
		t.Errorf("attachment not downloaded: %v", err)
	}
	imagesMD, err := os.ReadFile(filepath.Join(home, "TEST-1", "images.md"))
	if err != nil {
		t.Fatalf("images.md not written: %v", err)
	}
	if !strings.Contains(string(imagesMD), "![shot.png](attachments/shot.png)") {
		t.Errorf("images.md = %q, want an image link for shot.png", imagesMD)
	}

	st, err := handoff.LoadStatus(home, "TEST-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if st.Entry(handoff.StageFetch).State != handoff.StatePass {
		t.Errorf("fetch stage state = %s, want pass", st.Entry(handoff.StageFetch).State)
	}

	// Second run: idempotent. The fake server's status is now "In
	// Progress" — no longer todo-class — so no second transition call
	// happens, and the already-pass fetch stage is not re-touched, yet
	// the command still succeeds and rewrites the artifact.
	stdout2, _, exitCode2 := runRoot(t, "ticket", "fetch", "TEST-1", "--home", home)
	if exitCode2 != 0 {
		t.Fatalf("second fetch: exitCode = %d, want 0\n%s", exitCode2, stdout2.String())
	}
	if fj.transitionCalls != 1 {
		t.Errorf("transitionCalls = %d, want 1 (no transition on the second, already-in-progress fetch)", fj.transitionCalls)
	}
	st2, err := handoff.LoadStatus(home, "TEST-1")
	if err != nil {
		t.Fatalf("LoadStatus (2nd): %v", err)
	}
	if st2.Entry(handoff.StageFetch).State != handoff.StatePass {
		t.Errorf("fetch stage state after 2nd fetch = %s, want pass", st2.Entry(handoff.StageFetch).State)
	}
}

func TestTicketUpdate_MarkerCreateThenEdit(t *testing.T) {
	home := t.TempDir()
	srv, fj := newFakeJiraServer(t, "To Do")
	writeTicketTestConfig(t, home, srv.URL)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	if _, _, exitCode := runRoot(t, "ticket", "update", "TEST-1", "--marker", "ship", "--comment", "first body", "--home", home); exitCode != 0 {
		t.Fatalf("first update: exitCode = %d, want 0", exitCode)
	}
	if fj.commentPosts != 1 || fj.commentPuts != 0 {
		t.Fatalf("after first update: posts=%d puts=%d, want posts=1 puts=0", fj.commentPosts, fj.commentPuts)
	}

	if _, _, exitCode := runRoot(t, "ticket", "update", "TEST-1", "--marker", "ship", "--comment", "second body", "--home", home); exitCode != 0 {
		t.Fatalf("second update: exitCode = %d, want 0", exitCode)
	}
	if fj.commentPosts != 1 || fj.commentPuts != 1 {
		t.Fatalf("after second update: posts=%d puts=%d, want posts=1 puts=1 (edit, not duplicate)", fj.commentPosts, fj.commentPuts)
	}
}

func TestTicketUpdate_RequiresStatusOrComment(t *testing.T) {
	home := t.TempDir()
	writeTicketTestConfig(t, home, "http://127.0.0.1:0")
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	_, _, exitCode := runRoot(t, "ticket", "update", "TEST-1", "--home", home)
	if exitCode != 2 {
		t.Errorf("exitCode = %d, want 2 (usage)", exitCode)
	}
}

func TestTicketFetch_MissingTokenIsTrackerAuthBeforeAnyRequest(t *testing.T) {
	home := t.TempDir()
	// Deliberately unreachable base_url: if buildTracker's env check
	// did not run before any HTTP call, this would surface as a
	// connection error instead of tracker_auth.
	writeTicketTestConfig(t, home, "http://127.0.0.1:0")
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "")

	stdout, _, exitCode := runRoot(t, "ticket", "fetch", "TEST-1", "--home", home)
	if exitCode != 5 {
		t.Fatalf("exitCode = %d, want 5 (tracker_auth)\n%s", exitCode, stdout.String())
	}
	var env struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if env.Error == nil || env.Error.Code != "tracker_auth" {
		t.Errorf("error = %+v, want code=tracker_auth", env.Error)
	}
}

func TestTicketFetch_SimulateWritesNothing(t *testing.T) {
	home := t.TempDir()
	srv, fj := newFakeJiraServer(t, "To Do")
	writeTicketTestConfig(t, home, srv.URL)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	stdout, _, exitCode := runRoot(t, "ticket", "fetch", "TEST-1", "--home", home, "--simulate")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	if fj.transitionCalls != 0 || fj.commentPosts != 0 {
		t.Errorf("simulate performed a write: transitionCalls=%d commentPosts=%d", fj.transitionCalls, fj.commentPosts)
	}
	if _, err := os.Stat(filepath.Join(home, "TEST-1", "ticket.json")); !os.IsNotExist(err) {
		t.Errorf("ticket.json should not exist after --simulate")
	}

	var env struct {
		Result struct {
			WouldTransitionTo *string `json:"would_transition_to"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if env.Result.WouldTransitionTo == nil || *env.Result.WouldTransitionTo != "In Progress" {
		t.Errorf("would_transition_to = %v, want \"In Progress\"", env.Result.WouldTransitionTo)
	}
}

func TestTicketUpdate_SimulateWritesNothing(t *testing.T) {
	home := t.TempDir()
	srv, fj := newFakeJiraServer(t, "To Do")
	writeTicketTestConfig(t, home, srv.URL)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	stdout, _, exitCode := runRoot(t, "ticket", "update", "TEST-1", "--status", "in_progress", "--comment", "would-be body", "--home", home, "--simulate")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	if fj.transitionCalls != 0 || fj.commentPosts != 0 {
		t.Errorf("simulate performed a write: transitionCalls=%d commentPosts=%d", fj.transitionCalls, fj.commentPosts)
	}
}

func TestTicketShow_NoWrites(t *testing.T) {
	home := t.TempDir()
	srv, fj := newFakeJiraServer(t, "To Do")
	writeTicketTestConfig(t, home, srv.URL)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	stdout, _, exitCode := runRoot(t, "ticket", "show", "TEST-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	if fj.transitionCalls != 0 || fj.commentPosts != 0 {
		t.Errorf("show performed a write: transitionCalls=%d commentPosts=%d", fj.transitionCalls, fj.commentPosts)
	}
	if _, err := os.Stat(filepath.Join(home, "TEST-1")); !os.IsNotExist(err) {
		t.Errorf("ticket dir should not exist after `ticket show`")
	}
}
