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
	want := []string{"./...", "./internal/foo/..."}
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
type fakeExecer struct {
	t       *testing.T
	fixture string
}

func (f fakeExecer) Run(_ context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	f.t.Helper()
	if tool != "gremlins" {
		f.t.Fatalf("unexpected tool %q", tool)
	}
	data, err := os.ReadFile(f.fixture)
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

	report, err := Tool{}.Run(context.Background(), repo, scope, fakeExecer{t: t, fixture: "testdata/pass.json"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 3 || report.Killed != 2 {
		t.Errorf("Run() = %+v", report)
	}
	if _, statErr := os.Stat(filepath.Join(dir, reportFile)); !os.IsNotExist(statErr) {
		t.Error("report file was not cleaned up after Run")
	}
}

func TestToolRunEmptyScope(t *testing.T) {
	report, err := Tool{}.Run(context.Background(), detect.Repo{Name: "svc"}, mutation.Scope{}, fakeExecer{t: t}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 0 {
		t.Errorf("Run(empty scope) = %+v, want zero report", report)
	}
}
