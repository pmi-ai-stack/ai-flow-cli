package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var runIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// eventRunIDs returns the run_id of every line in an events.jsonl.
func eventRunIDs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var ids []string
	for line := range bytes.SplitSeq(bytes.TrimRight(data, "\n"), []byte("\n")) {
		var e struct {
			RunID string `json:"run_id"`
			Type  string `json:"type"`
		}
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("events line is not JSON: %v\n%s", err, line)
		}
		ids = append(ids, e.RunID)
	}
	return ids
}

// The point of the run id: a line on stderr and the events that
// invocation wrote to the handoff directory carry the same one.
func TestEventsJoinToTheStderrLogByRunID(t *testing.T) {
	home := t.TempDir()
	srv, _ := newFakeJiraServer(t, "To Do")
	writeTicketTestConfig(t, home, srv.URL)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	stdout, stderr, ec := runRoot(t, "ticket", "fetch", "TEST-1", "--home", home, "--verbose", "--log-format", "json")
	if ec != 0 {
		t.Fatalf("ticket fetch: exitCode = %d\n%s", ec, stdout.String())
	}

	logIDs := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(stderr.String()), "\n") {
		var m struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("stderr line is not JSON: %v\n%s", err, line)
		}
		logIDs[m.RunID] = true
	}
	if len(logIDs) != 1 {
		t.Fatalf("stderr carries %d run ids, want 1: %v", len(logIDs), logIDs)
	}

	ids := eventRunIDs(t, filepath.Join(home, "TEST-1", "events.jsonl"))
	if len(ids) == 0 {
		t.Fatal("ticket fetch wrote no events")
	}
	for _, id := range ids {
		if !logIDs[id] {
			t.Errorf("event run_id %q is not the invocation's stderr run id %v", id, logIDs)
		}
	}
}

// Without --verbose there is no stderr log, but the events still say
// which invocation wrote them, and two invocations differ.
func TestEventsCarryARunIDWithoutVerbose(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "PROJ-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, _, ec := runRoot(t, "status", "PROJ-1", "--home", home); ec != 0 {
			t.Fatalf("status: exitCode = %d", ec)
		}
	}
	ids := eventRunIDs(t, filepath.Join(home, "PROJ-1", "events.jsonl"))
	if len(ids) != 2 {
		t.Fatalf("got %d events, want 2", len(ids))
	}
	for _, id := range ids {
		if !runIDPattern.MatchString(id) {
			t.Errorf("run_id %q is not 16 hex digits", id)
		}
	}
	if ids[0] == ids[1] {
		t.Errorf("two invocations share run id %q", ids[0])
	}
}

func TestGuardDecisionEventCarriesARunID(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/plugin")
	home := t.TempDir()
	repo := filepath.Join(home, "..", "acme.billing.api")
	if err := os.MkdirAll(filepath.Join(home, "PROJ-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeActiveFixture(t, home, "PROJ-1", "ship", []string{repo})
	if _, _, ec := runRootStdin(t, bashPushPayloadFor(repo), "hook", "guard", "--home", home); ec != 0 {
		t.Fatalf("hook guard: exitCode = %d", ec)
	}
	ids := eventRunIDs(t, filepath.Join(home, "PROJ-1", "events.jsonl"))
	if len(ids) != 1 || !runIDPattern.MatchString(ids[0]) {
		t.Fatalf("guard decision run ids = %v, want one 16-hex id", ids)
	}
}

func TestDoctorVerboseLogsItsProbes(t *testing.T) {
	home := t.TempDir()
	writeRoutingTestConfig(t, home, t.TempDir(), false)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")
	useScriptedJira(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/rest/api/2/myself":
			return jsonResponse(200, `{"accountId":"1"}`), nil
		case "/rest/api/2/status":
			return jsonResponse(200, `[{"name":"To Do"},{"name":"In Progress"},{"name":"In Review"}]`), nil
		}
		t.Fatalf("unexpected request %s", r.URL.Path)
		return nil, nil
	})

	_, stderr, ec := runRoot(t, "doctor", "--home", home, "--verbose", "--log-format", "json")
	if ec != 0 {
		t.Fatalf("doctor: exitCode = %d", ec)
	}
	var sawMyself bool
	for line := range strings.SplitSeq(strings.TrimSpace(stderr.String()), "\n") {
		var m struct {
			Msg       string `json:"msg"`
			Component string `json:"component"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("stderr line is not JSON: %v\n%s", err, line)
		}
		if m.Component == "jira" && strings.HasPrefix(m.Msg, "GET /rest/api/2/myself") {
			sawMyself = true
		}
	}
	if !sawMyself {
		t.Errorf("doctor --verbose did not log its Jira probe:\n%s", stderr.String())
	}
}
