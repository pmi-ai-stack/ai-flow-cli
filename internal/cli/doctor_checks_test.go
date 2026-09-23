package cli

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// TestDoctorReportsMissingCheckRunners proves the phase-14
// growing-surfaces line: doctor names every tool an effective check
// resolves to that is not on this machine's PATH. It does not assert
// against any *particular* tool's real-environment presence — that is
// exactly what this test must not assume, since a machine running
// this suite may legitimately have any of gofmt/golangci-lint/gitleaks
// installed for other reasons (this repository's own dogfooding CI
// step, for one — see .ai-flow/standards/'s own doc comment). Instead
// it stubs exec.Which itself, so the assertion is about
// doctorProbeCheckRunners' own logic, not this machine's ambient PATH.
func TestDoctorReportsMissingCheckRunners(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-5")
	runStage(t, home, 0, "start", "PROJ-5", "implement")

	orig := whichOverride
	whichOverride = func(name string) (string, bool) {
		if name == "gofmt" {
			return "/usr/bin/gofmt", true
		}
		return "", false
	}
	t.Cleanup(func() { whichOverride = orig })

	stdout, _, ec := runRoot(t, "doctor", "--home", home)
	if ec != 0 {
		t.Fatalf("doctor: exitCode = %d\n%s", ec, stdout.String())
	}
	var env struct {
		Result struct {
			MissingCheckRunners []struct {
				CheckID string `json:"check_id"`
				Tool    string `json:"tool"`
			} `json:"missing_check_runners"`
		} `json:"result"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}

	var tools []string
	for _, m := range env.Result.MissingCheckRunners {
		tools = append(tools, m.Tool)
	}
	sort.Strings(tools)

	wantMissing := map[string]bool{"golangci-lint": true, "gitleaks": true}
	for tool := range wantMissing {
		found := false
		for _, got := range tools {
			if got == tool {
				found = true
			}
		}
		if !found {
			t.Errorf("missing_check_runners = %v, want %q among them", tools, tool)
		}
	}
	for _, tool := range tools {
		if tool == "gofmt" {
			t.Error("missing_check_runners lists gofmt, which the stub reports as found")
		}
	}

	var warnedGolangciLint bool
	for _, w := range env.Warnings {
		if strings.Contains(w, "golangci-lint") && strings.Contains(w, "not found on PATH") {
			warnedGolangciLint = true
		}
	}
	if !warnedGolangciLint {
		t.Errorf("warnings = %v, want one naming golangci-lint not found on PATH", env.Warnings)
	}
}

func TestDoctorNoActiveTicketReportsNoMissingCheckRunners(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	stdout, _, ec := runRoot(t, "doctor", "--home", home)
	if ec != 0 {
		t.Fatalf("doctor: exitCode = %d\n%s", ec, stdout.String())
	}
	var env struct {
		Result struct {
			MissingCheckRunners []any `json:"missing_check_runners"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(env.Result.MissingCheckRunners) != 0 {
		t.Errorf("missing_check_runners = %v, want none with no active ticket", env.Result.MissingCheckRunners)
	}
}
