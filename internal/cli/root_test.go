package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

var fixedBuildInfo = BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-09-21T00:00:00Z"}

// TestVersionGoldenJSON runs `ai-flow version` through the real root
// command and checks the printed envelope field by field. go_version is
// compared against runtime.Version() rather than a literal, since it is
// inherently tied to the toolchain that built the test binary — for a
// fixed toolchain this is still byte-stable.
func TestVersionGoldenJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := -1
	root := NewRoot(fixedBuildInfo, strings.NewReader(""), &stdout, &stderr, &exitCode)
	root.SetArgs([]string{"version"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}

	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Result  struct {
			Version       string `json:"version"`
			Commit        string `json:"commit"`
			Date          string `json:"date"`
			GoVersion     string `json:"go_version"`
			SchemaVersion int    `json:"schema_version"`
		} `json:"result"`
		NeedsInput *struct{} `json:"needs_input"`
		Error      *struct{} `json:"error"`
		Warnings   []string  `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}

	if !env.OK {
		t.Error("ok = false, want true")
	}
	if env.Command != "version" {
		t.Errorf("command = %q, want %q", env.Command, "version")
	}
	if env.Result.Version != "1.2.3" || env.Result.Commit != "abc123" || env.Result.Date != "2026-09-21T00:00:00Z" {
		t.Errorf("result = %+v", env.Result)
	}
	if env.Result.GoVersion != runtime.Version() {
		t.Errorf("go_version = %q, want %q", env.Result.GoVersion, runtime.Version())
	}
	if env.Result.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", env.Result.SchemaVersion)
	}
	if env.NeedsInput != nil {
		t.Error("needs_input should be null on success")
	}
	if env.Error != nil {
		t.Error("error should be null on success")
	}
	if env.Warnings == nil {
		t.Error("warnings should be [] (present, empty), not null")
	}

	if !strings.HasSuffix(stdout.String(), "\n") || strings.HasSuffix(stdout.String(), "\n\n") {
		t.Errorf("stdout does not end with exactly one trailing newline: %q", stdout.String())
	}
}

func TestVersionGoldenHuman(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := -1
	root := NewRoot(fixedBuildInfo, strings.NewReader(""), &stdout, &stderr, &exitCode)
	root.SetArgs([]string{"version", "--human"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "version: ok\n") {
		t.Errorf("human output does not start with %q, got %q", "version: ok\n", out)
	}
	for _, want := range []string{`"version": "1.2.3"`, `"commit": "abc123"`, `"schema_version": 1`} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
}

func TestVersionRunTwiceIsIdempotent(t *testing.T) {
	run := func() string {
		var stdout, stderr bytes.Buffer
		exitCode := -1
		root := NewRoot(fixedBuildInfo, strings.NewReader(""), &stdout, &stderr, &exitCode)
		root.SetArgs([]string{"version"})
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		return stdout.String()
	}
	if a, b := run(), run(); a != b {
		t.Errorf("two runs produced different bytes:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}

// TestGlobalFlagsReachContext proves --home/--simulate/--verbose/--human
// reach a subcommand's Flags through the context, not through
// package-level state. version has no visible side effect to assert on,
// so this drives a test-only command registered directly on the root
// that reports the Flags it observed.
func TestGlobalFlagsReachContext(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := -1
	root := NewRoot(fixedBuildInfo, strings.NewReader(""), &stdout, &stderr, &exitCode)

	var captured Flags
	root.AddCommand(&cobra.Command{
		Use: "capture-flags-for-test",
		RunE: wrap("capture-flags-for-test", &stdout, &exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			captured = FlagsFrom(cmd.Context())
			return Outcome{Result: map[string]any{}}, nil
		}),
	})
	root.SetArgs([]string{"capture-flags-for-test", "--home", "/tmp/custom-home", "--simulate", "--verbose", "--human"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if captured.Home != "/tmp/custom-home" {
		t.Errorf("Home = %q, want %q", captured.Home, "/tmp/custom-home")
	}
	if !captured.Simulate {
		t.Error("Simulate = false, want true")
	}
	if !captured.Verbose {
		t.Error("Verbose = false, want true")
	}
	if !captured.Human {
		t.Error("Human = false, want true")
	}
}

// TestWrapCarriesNeedsInputOnErrorPath proves the phase-5 fix to wrap:
// a handler returning both a non-nil *envelope.Error and a populated
// Outcome.NeedsInput gets both on the printed envelope — the shape
// routing_ambiguous, dirty_worktree and base_diverged all need.
// Before this fix, needs_input was only ever copied from Outcome on
// the success path.
func TestWrapCarriesNeedsInputOnErrorPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := -1
	root := NewRoot(fixedBuildInfo, strings.NewReader(""), &stdout, &stderr, &exitCode)

	root.AddCommand(&cobra.Command{
		Use: "needs-input-for-test",
		RunE: wrap("needs-input-for-test", &stdout, &exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			return Outcome{
				NeedsInput: &envelope.NeedsInput{
					Question:   "Which repositories does PROJ-1 touch?",
					Options:    []string{"acme.billing.api", "acme.billing.web"},
					ResumeWith: "repo confirm PROJ-1 --repos <REPO>",
				},
			}, &envelope.Error{Code: "routing_ambiguous", Message: "routing is ambiguous"}
		}),
	})
	root.SetArgs([]string{"needs-input-for-test"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 4 {
		t.Errorf("exitCode = %d, want 4", exitCode)
	}

	var env struct {
		OK         bool `json:"ok"`
		NeedsInput *struct {
			Question   string   `json:"question"`
			Options    []string `json:"options"`
			ResumeWith string   `json:"resume_with"`
		} `json:"needs_input"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.OK {
		t.Error("ok = true, want false")
	}
	if env.Error == nil || env.Error.Code != "routing_ambiguous" {
		t.Errorf("error = %+v, want code routing_ambiguous", env.Error)
	}
	if env.NeedsInput == nil {
		t.Fatal("needs_input = null, want it populated alongside the error")
	}
	if env.NeedsInput.Question != "Which repositories does PROJ-1 touch?" {
		t.Errorf("needs_input.question = %q", env.NeedsInput.Question)
	}
	if len(env.NeedsInput.Options) != 2 {
		t.Errorf("needs_input.options = %v, want 2 entries", env.NeedsInput.Options)
	}
	if env.NeedsInput.ResumeWith != "repo confirm PROJ-1 --repos <REPO>" {
		t.Errorf("needs_input.resume_with = %q", env.NeedsInput.ResumeWith)
	}
}
