package e2e_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/e2e"
)

func fixtureMatch() detect.E2EMatch {
	return detect.E2EMatch{Suite: "playwright", Command: "npx", Args: []string{"playwright", "test", "--reporter=json"}}
}

func TestRunNoBaseline(t *testing.T) {
	data, err := os.ReadFile("testdata/playwright/report.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).OnFailWithOutput(
		"npx", []string{"playwright", "test", "--reporter=json"},
		exec.Output{Stdout: string(data), ExitCode: 1},
		&envelope.Error{Code: "tool_failed"},
	)
	repo := detect.Repo{Name: "web", LocalPath: t.TempDir()}

	r, err := e2e.Run(context.Background(), repo, fixtureMatch(), "", nil, false, s, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Total != 3 || r.Failed != 1 {
		t.Errorf("Run = %+v, want total=3 failed=1", r)
	}
	if len(r.NewCases) != 0 {
		t.Errorf("NewCases = %v, want empty on the first call (nothing to compare against)", r.NewCases)
	}
}

func TestRunHasPreviousButEmptyReportsEveryCaseNew(t *testing.T) {
	data, err := os.ReadFile("testdata/playwright/report.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).OnFailWithOutput(
		"npx", []string{"playwright", "test", "--reporter=json"},
		exec.Output{Stdout: string(data), ExitCode: 1},
		&envelope.Error{Code: "tool_failed"},
	)
	repo := detect.Repo{Name: "web", LocalPath: t.TempDir()}

	// hasPrevious=true with a zero-length (but non-nil-meaning) baseline
	// is different from hasPrevious=false: a real prior e2e-results.json
	// that recorded zero cases means every current case is genuinely new.
	r, err := e2e.Run(context.Background(), repo, fixtureMatch(), "", []string{}, true, s, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !slices.Equal(r.NewCases, r.CaseNames) {
		t.Errorf("NewCases = %v, want every current case (%v)", r.NewCases, r.CaseNames)
	}
}

func TestRunNewCasesAgainstBaseline(t *testing.T) {
	data, err := os.ReadFile("testdata/playwright/report.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).OnFailWithOutput(
		"npx", []string{"playwright", "test", "--reporter=json"},
		exec.Output{Stdout: string(data), ExitCode: 1},
		&envelope.Error{Code: "tool_failed"},
	)
	repo := detect.Repo{Name: "web", LocalPath: t.TempDir()}

	// Simulates the base-ref run having seen only one of the three
	// cases — the branch run (this call) should report the other two
	// as new.
	previous := []string{"login > logs in with valid credentials"}
	r, err := e2e.Run(context.Background(), repo, fixtureMatch(), "", previous, true, s, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"login > password reset > sends a reset email", "login > rejects invalid credentials"}
	if !slices.Equal(r.NewCases, want) {
		t.Errorf("NewCases = %v, want %v", r.NewCases, want)
	}
}

func TestRunNoNewCasesWhenBaselineIsSuperset(t *testing.T) {
	data, err := os.ReadFile("testdata/playwright/report.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).OnFailWithOutput(
		"npx", []string{"playwright", "test", "--reporter=json"},
		exec.Output{Stdout: string(data), ExitCode: 1},
		&envelope.Error{Code: "tool_failed"},
	)
	repo := detect.Repo{Name: "web", LocalPath: t.TempDir()}

	previous := []string{
		"login > logs in with valid credentials",
		"login > rejects invalid credentials",
		"login > password reset > sends a reset email",
		"login > a case removed since",
	}
	r, err := e2e.Run(context.Background(), repo, fixtureMatch(), "", previous, true, s, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(r.NewCases) != 0 {
		t.Errorf("NewCases = %v, want empty when every current case was already in the baseline", r.NewCases)
	}
}

func TestRunAppendsGrepFlag(t *testing.T) {
	data, err := os.ReadFile("testdata/playwright/report.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).On(
		"npx", []string{"playwright", "test", "--reporter=json", "--grep", "login"},
		exec.Output{Stdout: string(data)},
	)
	repo := detect.Repo{Name: "web", LocalPath: t.TempDir()}

	if _, err := e2e.Run(context.Background(), repo, fixtureMatch(), "login", nil, false, s, time.Minute); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunExecNotAllowedPropagates(t *testing.T) {
	s := exectest.New(t).OnFail(
		"npx", []string{"playwright", "test", "--reporter=json"},
		exec.NotAllowed([]string{"npx", "playwright", "test", "--reporter=json"}),
	)
	repo := detect.Repo{Name: "web", LocalPath: t.TempDir()}

	if _, err := e2e.Run(context.Background(), repo, fixtureMatch(), "", nil, false, s, time.Minute); err == nil {
		t.Fatal("Run: want an error when the tool is not allowed")
	}
}
