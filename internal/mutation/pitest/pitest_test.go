package pitest

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestFqnFor(t *testing.T) {
	fqn, ok := fqnFor("src/main/java/com/acme/Foo.java")
	if !ok || fqn != "com.acme.Foo" {
		t.Errorf("fqnFor = %q, %v", fqn, ok)
	}
	if _, ok := fqnFor("scripts/build.java"); ok {
		t.Error("fqnFor(no src/main/java segment) = ok, want false")
	}
}

func TestTargetClasses(t *testing.T) {
	got := targetClasses([]string{
		"src/main/java/com/acme/Foo.java",
		"src/main/java/com/acme/Bar.java",
		"src/main/java/com/acme/Foo.java",
		"build.gradle",
	})
	want := []string{"com.acme.Bar", "com.acme.Foo"}
	if !slices.Equal(got, want) {
		t.Errorf("targetClasses = %v, want %v", got, want)
	}
}

func TestParsePass(t *testing.T) {
	data, err := os.ReadFile("testdata/pass.xml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.Killed != 2 || len(got.Survivors) != 0 {
		t.Errorf("Parse(pass) = %+v", got)
	}
}

func TestParseBelowThreshold(t *testing.T) {
	data, err := os.ReadFile("testdata/below_threshold.xml")
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
	if got.Survivors[0].File != "Foo.java" || got.Survivors[0].Line != 50 {
		t.Errorf("Survivors[0] = %+v", got.Survivors[0])
	}
}

func TestParseMalformed(t *testing.T) {
	if _, err := Parse([]byte("not xml")); err == nil {
		t.Error("Parse(malformed) = nil error, want one")
	}
}

type fakeExecer struct {
	t       *testing.T
	fixture string
}

func (f fakeExecer) Run(_ context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	if err := exectest.Refused(tool, args); err != nil {
		return exec.Output{}, err // what the real Runner answers (phase 54)
	}
	f.t.Helper()
	if tool != "mvn" {
		f.t.Fatalf("unexpected tool %q", tool)
	}
	data, err := os.ReadFile(f.fixture)
	if err != nil {
		f.t.Fatal(err)
	}
	out := filepath.Join(opts.Dir, reportsDir, "20260922120000", "mutations.xml")
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(out, data, 0o600); err != nil {
		f.t.Fatal(err)
	}
	return exec.Output{}, nil
}

func TestToolRun(t *testing.T) {
	dir := t.TempDir()
	repo := detect.Repo{Name: "svc", LocalPath: dir}
	scope := mutation.Scope{Repo: "svc", Files: []string{"src/main/java/com/acme/Foo.java"}}

	report, err := Tool{}.Run(context.Background(), repo, scope, fakeExecer{t: t, fixture: "testdata/pass.xml"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 2 || report.Killed != 2 {
		t.Errorf("Run() = %+v", report)
	}
	if _, statErr := os.Stat(filepath.Join(dir, reportsDir)); !os.IsNotExist(statErr) {
		t.Error("report directory was not cleaned up after Run")
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

func TestToolRunNoMavenFiles(t *testing.T) {
	// Every scope file is dropped by fqnFor (design decision 13) — Run
	// must not invoke mvn at all.
	scope := mutation.Scope{Repo: "svc", Files: []string{"scripts/build.java"}}
	report, err := Tool{}.Run(context.Background(), detect.Repo{Name: "svc"}, scope, fakeExecer{t: t}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 0 {
		t.Errorf("Run(no maven-convention files) = %+v, want zero report", report)
	}
}
