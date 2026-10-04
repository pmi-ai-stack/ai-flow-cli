package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// jiraFixture is one home and fake Jira shared by every run in a test,
// so stdout is comparable byte for byte between runs.
type jiraFixture struct{ home string }

func newJiraFixture(t *testing.T) jiraFixture {
	t.Helper()
	home := t.TempDir()
	srv, _ := newFakeJiraServer(t, "To Do")
	writeTicketTestConfig(t, home, srv.URL)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")
	return jiraFixture{home: home}
}

// ticketShow runs `ticket show TEST-1` with extra global flags.
func (f jiraFixture) ticketShow(t *testing.T, extra ...string) (stdout, stderr string) {
	t.Helper()
	args := append([]string{"ticket", "show", "TEST-1", "--home", f.home}, extra...)
	so, se, ec := runRoot(t, args...)
	if ec != 0 {
		t.Fatalf("%v: exitCode = %d\n%s", args, ec, so.String())
	}
	return so.String(), se.String()
}

func TestVerboseJSONLogsCarryOneRunID(t *testing.T) {
	_, stderr := newJiraFixture(t).ticketShow(t, "--verbose", "--log-format", "json")
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("--verbose --log-format json wrote nothing to stderr")
	}
	runIDs := map[string]bool{}
	var sawGet bool
	for _, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("stderr line is not JSON: %v\n%s", err, line)
		}
		runIDs[m["run_id"].(string)] = true
		if m["command"] != "ticket show" {
			t.Errorf("command = %v, want \"ticket show\"", m["command"])
		}
		if msg, _ := m["msg"].(string); strings.HasPrefix(msg, "GET /rest/api/2/issue/TEST-1") && m["component"] == "jira" {
			sawGet = true
		}
	}
	if len(runIDs) != 1 {
		t.Errorf("lines carry %d different run ids, want exactly one: %v", len(runIDs), runIDs)
	}
	if !sawGet {
		t.Errorf("no jira GET line for TEST-1 in:\n%s", stderr)
	}
}

func TestVerboseTextKeepsTheHumanLines(t *testing.T) {
	_, stderr := newJiraFixture(t).ticketShow(t, "--verbose")
	if !strings.Contains(stderr, "GET /rest/api/2/issue/TEST-1") {
		t.Fatalf("stderr = %q, want the GET line", stderr)
	}
	if strings.HasPrefix(strings.TrimSpace(stderr), "{") {
		t.Errorf("text format wrote JSON:\n%s", stderr)
	}
}

func TestLogFormatAloneWritesNothing(t *testing.T) {
	_, stderr := newJiraFixture(t).ticketShow(t, "--log-format", "json")
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("without --verbose stderr = %q, want empty", stderr)
	}
}

// The envelope is the machine contract: neither flag may change a byte
// of it.
func TestLoggingFlagsDoNotChangeStdout(t *testing.T) {
	fx := newJiraFixture(t)
	plain, _ := fx.ticketShow(t)
	for _, extra := range [][]string{{"--verbose"}, {"--verbose", "--log-format", "json"}, {"--log-format", "json"}} {
		got, _ := fx.ticketShow(t, extra...)
		if got != plain {
			t.Errorf("stdout differs with %v:\n--- plain\n%s\n--- with flags\n%s", extra, plain, got)
		}
	}
}

func TestBadLogFormatIsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ec := 0
	root := NewRoot(fixedBuildInfo, strings.NewReader(""), &stdout, &stderr, &ec)
	root.SetArgs([]string{"version", "--log-format", "yaml"})
	err := root.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "--log-format") {
		t.Fatalf("err = %v, want a rejection naming --log-format", err)
	}
}
