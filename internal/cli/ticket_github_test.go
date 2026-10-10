package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// writeGitHubTrackerConfig is writeTicketTestConfig for provider
// "github": prefix CLI names acme/widgets, statuses are labels.
func writeGitHubTrackerConfig(t *testing.T, home, baseURL string) {
	t.Helper()
	cfg := fmt.Sprintf(`{
  "schema_version": 1,
  "tracker": {
    "provider": "github",
    "base_url": %q,
    "statuses": { "todo": ["open"], "in_progress": "in progress", "in_review": "in review" },
    "bug_types": ["Bug"],
    "repositories": { "CLI": "acme/widgets" },
    "comment_limit": 5
  },
  "host": { "provider": "github", "default_base_branch": "main", "draft_prs": false },
  "routing": { "project_key_map": {}, "keyword_to_repo": {}, "teams": {} },
  "defaults": { "worktree": false, "viewport": "1440x900", "mutation_threshold": 80, "mutation_max_rounds": 3, "mutation_timeout_minutes": 20, "test_timeout_minutes": 20, "guard_idle_hours": 8 },
  "standards": { "sources": [], "guidance_budget_tokens": 6000 },
  "ops": { "environments": {}, "target_map": {}, "default_environment": "", "risk_gate": 40, "risk": { "action_weight": {}, "kind_weight": {}, "tier_multiplier": {}, "drift_penalty": 5, "offline_penalty": 10 }, "protected_kinds": [], "e2e": { "enabled": false, "sandbox_environment": "" }, "tool_timeout_minutes": 30 }
}`, baseURL)
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeGitHubIssues is an in-memory GitHub Issues double: issue
// acme/widgets#7, open, labelled bug, with one comment by "octo".
type fakeGitHubIssues struct {
	mu        sync.Mutex
	labels    []string
	comments  []fakeComment
	writes    []string
	nextID    int
	authorize string
}

func newFakeGitHubIssuesServer(t *testing.T) (*httptest.Server, *fakeGitHubIssues) {
	t.Helper()
	fg := &fakeGitHubIssues{labels: []string{"bug"}, comments: []fakeComment{{id: "100", author: "octo", body: "first"}}, nextID: 101}
	mux := http.NewServeMux()
	issue := "/repos/acme/widgets/issues/7"

	mux.HandleFunc("GET "+issue, func(w http.ResponseWriter, r *http.Request) {
		fg.mu.Lock()
		defer fg.mu.Unlock()
		fg.authorize = r.Header.Get("Authorization")
		labels := make([]map[string]string, len(fg.labels))
		for i, l := range fg.labels {
			labels[i] = map[string]string{"name": l}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 7, "title": "Widget crashes", "body": "Steps…", "state": "open", "labels": labels,
		})
	})
	mux.HandleFunc("GET "+issue+"/comments", func(w http.ResponseWriter, r *http.Request) {
		fg.mu.Lock()
		defer fg.mu.Unlock()
		out := []map[string]any{}
		for _, c := range fg.comments {
			id := 0
			_, _ = fmt.Sscan(c.id, &id)
			out = append(out, map[string]any{"id": id, "user": map[string]string{"login": c.author}, "body": c.body, "created_at": "2026-10-01T00:00:00Z"})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("POST "+issue+"/labels", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Labels []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		fg.mu.Lock()
		defer fg.mu.Unlock()
		fg.writes = append(fg.writes, "add "+strings.Join(body.Labels, ","))
		for _, l := range body.Labels {
			if !slices.Contains(fg.labels, l) {
				fg.labels = append(fg.labels, l)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("DELETE "+issue+"/labels/{name}", func(w http.ResponseWriter, r *http.Request) {
		fg.mu.Lock()
		defer fg.mu.Unlock()
		name := r.PathValue("name")
		fg.writes = append(fg.writes, "remove "+name)
		fg.labels = slices.DeleteFunc(fg.labels, func(l string) bool { return l == name })
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST "+issue+"/comments", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Body string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		fg.mu.Lock()
		defer fg.mu.Unlock()
		fg.writes = append(fg.writes, "comment")
		fg.comments = append(fg.comments, fakeComment{id: fmt.Sprint(fg.nextID), author: "ai-flow", body: body.Body})
		fg.nextID++
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("PATCH /repos/acme/widgets/issues/comments/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Body string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		fg.mu.Lock()
		defer fg.mu.Unlock()
		fg.writes = append(fg.writes, "edit "+r.PathValue("id"))
		for i, c := range fg.comments {
			if c.id == r.PathValue("id") {
				fg.comments[i].body = body.Body
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"login":"ai-flow-bot"}`)
	})
	mux.HandleFunc("GET /repos/acme/widgets/labels", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"bug"},{"name":"in progress"},{"name":"in review"}]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, fg
}

func useGitHubTracker(t *testing.T) (home string, fg *fakeGitHubIssues) {
	t.Helper()
	home = t.TempDir()
	srv, fg := newFakeGitHubIssuesServer(t)
	writeGitHubTrackerConfig(t, home, srv.URL)
	t.Setenv("AI_FLOW_GITHUB_TOKEN", "gh-test-token")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	return home, fg
}

// TestTicketFetchGitHubIssue: the same `ticket fetch` against GitHub
// Issues — the bug label makes a bugfix branch, the untouched open
// issue is moved to "in progress" by label, comments are kept, and a
// second fetch writes nothing.
func TestTicketFetchGitHubIssue(t *testing.T) {
	home, fg := useGitHubTracker(t)

	stdout, _, ec := runRoot(t, "ticket", "fetch", "CLI-7", "--home", home)
	if ec != 0 {
		t.Fatalf("exit %d\n%s", ec, stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(home, "CLI-7", "ticket.json"))
	if err != nil {
		t.Fatal(err)
	}
	var art struct {
		Project     string   `json:"project"`
		IssueType   string   `json:"issue_type"`
		Summary     string   `json:"summary"`
		Status      string   `json:"status"`
		BranchName  string   `json:"branch_name"`
		Attachments []string `json:"attachments"`
		Comments    []struct {
			Author string `json:"author"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(data, &art); err != nil {
		t.Fatal(err)
	}
	if art.Project != "CLI" || art.IssueType != "Bug" || art.Summary != "Widget crashes" || art.Status != "in progress" ||
		art.BranchName != "bugfix/CLI-7" || len(art.Attachments) != 0 || len(art.Comments) != 1 || art.Comments[0].Author != "octo" {
		t.Errorf("ticket.json = %s", data)
	}
	if fg.authorize != "Bearer gh-test-token" {
		t.Errorf("Authorization = %q", fg.authorize)
	}

	if _, _, ec := runRoot(t, "ticket", "fetch", "CLI-7", "--home", home); ec != 0 {
		t.Fatalf("second fetch exit %d", ec)
	}
	if !slices.Equal(fg.writes, []string{"add in progress"}) {
		t.Errorf("writes = %v, want one label add across both fetches", fg.writes)
	}
}

// TestTicketUpdateGitHubIssue: in_review swaps the status labels, and a
// marked comment is posted once, then edited in place.
func TestTicketUpdateGitHubIssue(t *testing.T) {
	home, fg := useGitHubTracker(t)
	if _, _, ec := runRoot(t, "ticket", "fetch", "CLI-7", "--home", home); ec != 0 {
		t.Fatal("fetch")
	}

	stdout, _, ec := runRoot(t, "ticket", "update", "CLI-7", "--home", home, "--status", "in_review", "--comment", "PR opened", "--marker", "pr")
	if ec != 0 {
		t.Fatalf("exit %d\n%s", ec, stdout.String())
	}
	if _, _, ec := runRoot(t, "ticket", "update", "CLI-7", "--home", home, "--comment", "PR updated", "--marker", "pr"); ec != 0 {
		t.Fatal("second update")
	}
	want := []string{"add in progress", "add in review", "remove in progress", "comment", "edit 101"}
	if !slices.Equal(fg.writes, want) {
		t.Errorf("writes = %v\nwant     %v", fg.writes, want)
	}
	if got := fg.comments[1].body; got != "<!-- ai-flow:pr -->\nPR updated" {
		t.Errorf("comment body = %q", got)
	}

	sim, _, ec := runRoot(t, "ticket", "update", "CLI-7", "--home", home, "--status", "in_progress", "--simulate")
	if ec != 0 || !strings.Contains(sim.String(), `"would_transition_to": "in progress"`) {
		t.Errorf("simulate: exit %d\n%s", ec, sim.String())
	}
}

func TestTicketFetchGitHubMissingTokenIsTrackerAuth(t *testing.T) {
	home, fg := useGitHubTracker(t)
	t.Setenv("AI_FLOW_GITHUB_TOKEN", "")
	stdout, _, ec := runRoot(t, "ticket", "fetch", "CLI-7", "--home", home)
	if ec != 5 || !strings.Contains(stdout.String(), `"code": "tracker_auth"`) || !strings.Contains(stdout.String(), "AI_FLOW_GITHUB_TOKEN") {
		t.Errorf("exit %d\n%s", ec, stdout.String())
	}
	if fg.authorize != "" {
		t.Error("a request was made without a token")
	}
}

// TestDoctorProbesGitHubTracker: doctor's tracker probe goes through
// the same factory — the GitHub token counts as the credentials, and
// the configured status labels (plus "open") are all found.
func TestDoctorProbesGitHubTracker(t *testing.T) {
	home, _ := useGitHubTracker(t)
	cfg, err := config.Load(home, "")
	if err != nil {
		t.Fatal(err)
	}
	dt := doctorProbeTracker(context.Background(), cfg)
	if !dt.CredsSet || dt.Reachable == nil || !*dt.Reachable || dt.Error != "" || len(dt.MissingStatuses) != 0 {
		t.Errorf("doctor tracker = %+v", dt)
	}
}
