package mutation

import (
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
)

func TestScopeForGremlins(t *testing.T) {
	repo := detect.Repo{Name: "acme.billing.api"}
	diff := []string{"main.go", "main_test.go", "internal/foo.go", "README.md"}
	got := ScopeFor(repo, "gremlins", diff)
	want := []string{"main.go", "internal/foo.go"}
	if !slices.Equal(got.Files, want) {
		t.Errorf("ScopeFor(gremlins) = %v, want %v", got.Files, want)
	}
	if got.Repo != "acme.billing.api" {
		t.Errorf("Repo = %q", got.Repo)
	}
}

func TestScopeForStryker(t *testing.T) {
	diff := []string{
		"src/foo.ts", "src/foo.test.ts", "src/bar.spec.tsx",
		"__tests__/baz.ts", "src/qux.js", "README.md",
	}
	got := ScopeFor(detect.Repo{Name: "web"}, "stryker", diff)
	want := []string{"src/foo.ts", "src/qux.js"}
	if !slices.Equal(got.Files, want) {
		t.Errorf("ScopeFor(stryker) = %v, want %v", got.Files, want)
	}
}

func TestScopeForMutmut(t *testing.T) {
	diff := []string{
		"pkg/add.py", "pkg/test_add.py", "pkg/sub_test.py",
		"tests/helpers.py", "README.md",
	}
	got := ScopeFor(detect.Repo{Name: "svc"}, "mutmut", diff)
	want := []string{"pkg/add.py"}
	if !slices.Equal(got.Files, want) {
		t.Errorf("ScopeFor(mutmut) = %v, want %v", got.Files, want)
	}
}

func TestScopeForPitest(t *testing.T) {
	diff := []string{
		"src/main/java/com/acme/Foo.java",
		"src/test/java/com/acme/FooTest.java",
		"src/main/java/com/acme/TestUtil.java",
		"README.md",
	}
	got := ScopeFor(detect.Repo{Name: "svc"}, "pitest", diff)
	want := []string{"src/main/java/com/acme/Foo.java"}
	if !slices.Equal(got.Files, want) {
		t.Errorf("ScopeFor(pitest) = %v, want %v", got.Files, want)
	}
}

func TestScopeForNoMatches(t *testing.T) {
	got := ScopeFor(detect.Repo{Name: "svc"}, "gremlins", []string{"README.md", "docs/plan.md"})
	if len(got.Files) != 0 {
		t.Errorf("ScopeFor with no mutable files = %v, want empty", got.Files)
	}
}

func TestScopeForUnknownTool(t *testing.T) {
	got := ScopeFor(detect.Repo{Name: "svc"}, "cosmic-ray", []string{"main.go"})
	if len(got.Files) != 0 {
		t.Errorf("ScopeFor with unknown tool = %v, want empty", got.Files)
	}
}
