package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

// useScriptedTransport swaps *target for the duration of the test,
// restoring the original afterward — the shared body behind
// useScriptedGitHub and doctor_test.go's useScriptedJira, both of which
// swap their own package-level http.RoundTripper source var the same
// way useScriptedExecer swaps newExecer.
func useScriptedTransport(t *testing.T, target *func() http.RoundTripper, handler func(r *http.Request) (*http.Response, error)) {
	t.Helper()
	orig := *target
	*target = func() http.RoundTripper { return roundTripFunc(handler) }
	t.Cleanup(func() { *target = orig })
}

// useScriptedGitHub swaps newGitHubTransport for the duration of the
// test, restoring the real one afterward.
func useScriptedGitHub(t *testing.T, handler func(r *http.Request) (*http.Response, error)) {
	t.Helper()
	useScriptedTransport(t, &newGitHubTransport, handler)
}

func setUpPRFixture(t *testing.T) (home, ticketDir string) {
	t.Helper()
	home = t.TempDir()
	localBase := t.TempDir()
	apiDir := repoInstructionsDir(t, localBase, "acme.billing.api")
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir = filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "Fix billing retry", "desc", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", LocalPath: apiDir, InstructionsFile: "AGENTS.md"},
	})
	if err := os.WriteFile(filepath.Join(ticketDir, "pr-body.md"), []byte("body text\n"), 0o600); err != nil {
		t.Fatalf("write pr-body.md: %v", err)
	}
	approveShip(t, home, "PROJ-1")
	return home, ticketDir
}

func TestPRCreate_RefusesWithoutShipApproval(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	writeRoutingTestConfig(t, home, localBase, false)
	writeTicketFixture(t, filepath.Join(home, "PROJ-1"), "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, filepath.Join(home, "PROJ-1"), "Team Beta", nil)

	_, _, exitCode := runRoot(t, "pr", "create", "PROJ-1", "--home", home)
	if exitCode != 3 {
		t.Fatalf("exitCode = %d, want 3", exitCode)
	}
}

func TestPRCreate_MissingTokenIsHostAuth(t *testing.T) {
	home, _ := setUpPRFixture(t)

	stdout, _, exitCode := runRoot(t, "pr", "create", "PROJ-1", "--home", home)
	if exitCode != 5 {
		t.Fatalf("exitCode = %d, want 5\n%s", exitCode, stdout.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.Error.Code != "host_auth" {
		t.Errorf("error.code = %q, want host_auth", env.Error.Code)
	}
}

func TestPRCreate_CreatesAndWritesShipResults(t *testing.T) {
	home, ticketDir := setUpPRFixture(t)
	t.Setenv("AI_FLOW_GITHUB_TOKEN", "tok")

	se := newScriptedExecer(t).
		On("git", []string{"push", "-u", "origin", "feature/PROJ-1"}, exec.Output{}).
		On("git", []string{"remote", "get-url", "origin"}, exec.Output{Stdout: "git@github.com:acme/billing-api.git\n"})
	useScriptedExecer(t, se)

	var postCount int
	var postedBody string
	useScriptedGitHub(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/acme/billing-api/pulls"):
			return jsonResponse(200, "[]"), nil
		case r.Method == "POST" && r.URL.Path == "/repos/acme/billing-api/pulls":
			postCount++
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			var reqBody struct {
				Body string `json:"body"`
			}
			if err := json.Unmarshal(raw, &reqBody); err != nil {
				t.Fatalf("request body not valid JSON: %v", err)
			}
			postedBody = reqBody.Body
			return jsonResponse(201, `{"number":7,"html_url":"https://github.com/acme/billing-api/pull/7","draft":false}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
			return nil, nil
		}
	})

	stdout, _, exitCode := runRoot(t, "pr", "create", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	if postCount != 1 {
		t.Errorf("POST called %d times, want 1", postCount)
	}
	if !strings.Contains(postedBody, "body text") {
		t.Errorf("PR body dropped the agent-authored pr-body.md content:\n%s", postedBody)
	}
	if !strings.Contains(postedBody, "## ai-flow summary") || !strings.Contains(postedBody, "acme.billing.api") {
		t.Errorf("PR body missing the generated summary block:\n%s", postedBody)
	}
	if strings.Contains(postedBody, "plan.md") {
		t.Error("PR body links plan.md, which is never part of the git repository a hosted PR is read from")
	}

	data, err := os.ReadFile(filepath.Join(ticketDir, "ship-results.json"))
	if err != nil {
		t.Fatalf("ship-results.json not written: %v", err)
	}
	var results shipResultsArtifact
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("ship-results.json invalid: %v", err)
	}
	if len(results.PullRequests) != 1 {
		t.Fatalf("pull_requests = %+v, want 1 entry", results.PullRequests)
	}
	pr := results.PullRequests[0]
	if pr.Repo != "acme.billing.api" || pr.URL != "https://github.com/acme/billing-api/pull/7" || pr.Number != 7 || pr.Reused {
		t.Errorf("pull_requests[0] = %+v", pr)
	}
}

func TestPRCreate_ReusesExistingOpenPRWithoutPosting(t *testing.T) {
	home, ticketDir := setUpPRFixture(t)
	t.Setenv("AI_FLOW_GITHUB_TOKEN", "tok")

	se := newScriptedExecer(t).
		On("git", []string{"push", "-u", "origin", "feature/PROJ-1"}, exec.Output{}).
		On("git", []string{"remote", "get-url", "origin"}, exec.Output{Stdout: "git@github.com:acme/billing-api.git\n"})
	useScriptedExecer(t, se)

	useScriptedGitHub(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/acme/billing-api/pulls") {
			return jsonResponse(200, `[{"number":42,"html_url":"https://github.com/acme/billing-api/pull/42","draft":false}]`), nil
		}
		// A POST here would mean EnsurePR failed to reuse — fail loudly.
		t.Fatalf("unexpected request %s %s (should have reused #42)", r.Method, r.URL.String())
		return nil, nil
	})

	stdout, _, exitCode := runRoot(t, "pr", "create", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(ticketDir, "ship-results.json"))
	if err != nil {
		t.Fatalf("ship-results.json not written: %v", err)
	}
	var results shipResultsArtifact
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("ship-results.json invalid: %v", err)
	}
	if len(results.PullRequests) != 1 || results.PullRequests[0].Number != 42 || !results.PullRequests[0].Reused {
		t.Errorf("pull_requests = %+v, want reused #42", results.PullRequests)
	}
}

func TestPRCreate_SimulateMakesNoPushOrHTTPCall(t *testing.T) {
	home, _ := setUpPRFixture(t)
	// No token set, no exec/http fixtures registered at all — a
	// simulate run must never reach either.
	se := newScriptedExecer(t).On("git", []string{"remote", "get-url", "origin"}, exec.Output{Stdout: "git@github.com:acme/billing-api.git\n"})
	useScriptedExecer(t, se)

	stdout, _, exitCode := runRoot(t, "pr", "create", "PROJ-1", "--home", home, "--simulate")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result prCreateSimulateResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if len(env.Result.PullRequests) != 1 || env.Result.PullRequests[0].Title != "PROJ-1: Fix billing retry" {
		t.Errorf("result = %+v", env.Result)
	}
}
