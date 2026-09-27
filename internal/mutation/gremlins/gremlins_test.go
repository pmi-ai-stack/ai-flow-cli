package gremlins

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

func TestParsePass(t *testing.T) {
	data, err := os.ReadFile("testdata/pass.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 3 || got.Killed != 2 || len(got.Survivors) != 0 {
		t.Errorf("Parse(pass) = %+v", got)
	}
}

func TestParseBelowThreshold(t *testing.T) {
	data, err := os.ReadFile("testdata/below_threshold.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.Killed != 1 || len(got.Survivors) != 1 {
		t.Errorf("Parse(below_threshold) = %+v", got)
	}
	if got.Survivors[0].File != "main.go" || got.Survivors[0].Line != 22 {
		t.Errorf("Survivors[0] = %+v", got.Survivors[0])
	}
}

func TestParseMalformed(t *testing.T) {
	if _, err := Parse([]byte("not json")); err == nil {
		t.Error("Parse(malformed) = nil error, want one")
	}
}

func TestPackageDirs(t *testing.T) {
	got := packageDirs([]string{"main.go", "internal/foo/bar.go", "internal/foo/baz.go"})
	want := []string{".", "./internal/foo"}
	if len(got) != len(want) {
		t.Fatalf("packageDirs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("packageDirs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// fakeExecer simulates gremlins: it writes fixture to the report file
// Tool.Run asks for, mirroring internal/verify/unit's pytestFakeExecer.
// It also records every invocation's own final positional argument (the
// one directory gremlins' real CLI accepts — see packageDirs' own doc
// comment) so TestToolRunAcrossMultipleDirectories can assert Run calls
// gremlins once per directory rather than once with every directory as
// a separate positional argument, the exact shape a real gremlins
// binary rejects with "accepts at most 1 arg(s)".
type fakeExecer struct {
	t        *testing.T
	fixture  string
	fixtures map[string]string // dir -> fixture path, for a multi-call test
	calls    []string          // each call's own single directory argument
}

func (f *fakeExecer) Run(_ context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	f.t.Helper()
	if tool != "gremlins" {
		f.t.Fatalf("unexpected tool %q", tool)
	}
	if len(args) != 4 || args[0] != "unleash" || args[1] != "--output" {
		f.t.Fatalf("args = %v, want [unleash --output <file> <one directory>]", args)
	}
	dir := args[3]
	f.calls = append(f.calls, dir)

	fixture := f.fixture
	if f.fixtures != nil {
		fixture = f.fixtures[dir]
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.Dir, reportFile), data, 0o600); err != nil {
		f.t.Fatal(err)
	}
	return exec.Output{}, nil
}

func TestToolRun(t *testing.T) {
	dir := t.TempDir()
	repo := detect.Repo{Name: "acme.billing.api", LocalPath: dir}
	scope := mutation.Scope{Repo: "acme.billing.api", Files: []string{"main.go"}}

	x := &fakeExecer{t: t, fixture: "testdata/pass.json"}
	report, err := Tool{}.Run(context.Background(), repo, scope, x, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 3 || report.Killed != 2 {
		t.Errorf("Run() = %+v", report)
	}
	if got := x.calls; len(got) != 1 || got[0] != "." {
		t.Errorf("calls = %v, want a single call with directory \".\"", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, reportFile)); !os.IsNotExist(statErr) {
		t.Error("report file was not cleaned up after Run")
	}
}

// TestToolRunAcrossMultipleDirectories is this fix's own regression
// test: two changed files in two different directories used to become
// two positional arguments in one gremlins invocation — real gremlins
// exits 2 on that ("accepts at most 1 arg(s)"). Run must instead call
// gremlins once per directory and merge the two reports, with
// Survivor IDs renumbered so both directories' own "gremlins-0" do not
// collide.
func TestToolRunAcrossMultipleDirectories(t *testing.T) {
	dir := t.TempDir()
	repo := detect.Repo{Name: "acme.billing.api", LocalPath: dir}
	scope := mutation.Scope{Repo: "acme.billing.api", Files: []string{"internal/foo/bar.go", "internal/baz/qux.go"}}

	x := &fakeExecer{t: t, fixtures: map[string]string{
		"./internal/foo": "testdata/pass.json",
		"./internal/baz": "testdata/below_threshold.json",
	}}
	report, err := Tool{}.Run(context.Background(), repo, scope, x, 0)
	if err != nil {
		t.Fatal(err)
	}

	wantCalls := []string{"./internal/baz", "./internal/foo"} // strutil.DedupSorted's own order
	if len(x.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", x.calls, wantCalls)
	}
	for i := range wantCalls {
		if x.calls[i] != wantCalls[i] {
			t.Errorf("calls[%d] = %q, want %q", i, x.calls[i], wantCalls[i])
		}
	}

	if report.Total != 5 || report.Killed != 3 {
		t.Errorf("merged report = %+v, want Total=5 Killed=3 (pass.json's 3/2 plus below_threshold.json's 2/1)", report)
	}
	if len(report.Survivors) != 1 {
		t.Fatalf("Survivors = %+v, want exactly below_threshold.json's own one survivor", report.Survivors)
	}
	if report.Survivors[0].ID != "gremlins-0" {
		t.Errorf("Survivors[0].ID = %q, want %q (renumbered after merge)", report.Survivors[0].ID, "gremlins-0")
	}
}

func TestToolRunEmptyScope(t *testing.T) {
	report, err := Tool{}.Run(context.Background(), detect.Repo{Name: "svc"}, mutation.Scope{}, &fakeExecer{t: t}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 0 {
		t.Errorf("Run(empty scope) = %+v, want zero report", report)
	}
}
