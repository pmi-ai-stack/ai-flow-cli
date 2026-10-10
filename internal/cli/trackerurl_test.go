package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// A plain-http tracker.base_url is refused before any credential is
// attached — config_invalid, not a connection attempt (2026-10-04
// security review #3). The host is unresolvable on purpose: had a
// request been made, the error would be tool_failed, exit 5.
func TestTicketFetch_PlainHTTPBaseURLRefusedBeforeAnyRequest(t *testing.T) {
	home := t.TempDir()
	writeTicketTestConfig(t, home, "http://jira.example.invalid")
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	stdout, _, exitCode := runRoot(t, "ticket", "fetch", "TEST-1", "--home", home)
	if exitCode != 6 {
		t.Fatalf("exitCode = %d, want 6 (config_invalid)\n%s", exitCode, stdout.String())
	}
	var env struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if env.Error == nil || env.Error.Code != "config_invalid" || !strings.Contains(env.Error.Message, "https://") {
		t.Errorf("error = %+v, want config_invalid naming https://", env.Error)
	}
}

func TestDoctorProbeTracker_PlainHTTPBaseURLNotProbed(t *testing.T) {
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")
	cfg := &config.Config{Tracker: config.Tracker{BaseURL: "http://jira.example.invalid"}}

	dt := doctorProbeTracker(context.Background(), cfg)
	if dt.Reachable != nil {
		t.Errorf("Reachable = %v, want nil — no request may be made", *dt.Reachable)
	}
	if !strings.Contains(dt.Error, "https://") {
		t.Errorf("Error = %q, want it to name https://", dt.Error)
	}
}
