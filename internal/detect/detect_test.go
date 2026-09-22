package detect_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
)

func fixtureRepo(t *testing.T, name string) detect.Repo {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "repos", name))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return detect.Repo{Name: name, LocalPath: abs}
}

func TestLanguages(t *testing.T) {
	cases := []struct {
		repo string
		want []string
	}{
		{"go", []string{"go"}},
		{"typescript", []string{"javascript", "typescript"}},
		{"python", []string{"python"}},
	}
	for _, c := range cases {
		t.Run(c.repo, func(t *testing.T) {
			got, err := detect.Languages(fixtureRepo(t, c.repo))
			if err != nil {
				t.Fatalf("Languages: %v", err)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("Languages(%s) = %v, want %v", c.repo, got, c.want)
			}
		})
	}
}

func TestLanguagesEmptyRepo(t *testing.T) {
	got, err := detect.Languages(detect.Repo{Name: "empty", LocalPath: t.TempDir()})
	if err != nil {
		t.Fatalf("Languages: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Languages(empty) = %v, want empty", got)
	}
}

func TestRunners(t *testing.T) {
	cases := []struct {
		repo       string
		wantRunner string
	}{
		{"go", "go"},
		{"typescript", "jest"},
		{"python", "pytest"},
	}
	for _, c := range cases {
		t.Run(c.repo, func(t *testing.T) {
			got, ok, err := detect.Runners(fixtureRepo(t, c.repo))
			if err != nil {
				t.Fatalf("Runners: %v", err)
			}
			if !ok {
				t.Fatalf("Runners(%s): ok = false, want true", c.repo)
			}
			if got.Runner != c.wantRunner {
				t.Errorf("Runners(%s).Runner = %q, want %q", c.repo, got.Runner, c.wantRunner)
			}
		})
	}
}

func TestRunnersNoMatch(t *testing.T) {
	_, ok, err := detect.Runners(detect.Repo{Name: "empty", LocalPath: t.TempDir()})
	if err != nil {
		t.Fatalf("Runners: %v", err)
	}
	if ok {
		t.Error("Runners(empty): ok = true, want false")
	}
}

func TestMutation(t *testing.T) {
	cases := []struct {
		repo string
		want string
	}{
		{"go", "gremlins"},
		{"typescript", "stryker"},
		{"python", "mutmut"},
	}
	for _, c := range cases {
		t.Run(c.repo, func(t *testing.T) {
			got, ok, err := detect.Mutation(fixtureRepo(t, c.repo))
			if err != nil {
				t.Fatalf("Mutation: %v", err)
			}
			if !ok {
				t.Fatalf("Mutation(%s): ok = false, want true", c.repo)
			}
			if got.Tool != c.want {
				t.Errorf("Mutation(%s).Tool = %q, want %q", c.repo, got.Tool, c.want)
			}
		})
	}
}

func TestMutationNoMatch(t *testing.T) {
	_, ok, err := detect.Mutation(detect.Repo{Name: "empty", LocalPath: t.TempDir()})
	if err != nil {
		t.Fatalf("Mutation: %v", err)
	}
	if ok {
		t.Error("Mutation(empty): ok = true, want false")
	}
}

func TestE2E(t *testing.T) {
	got, ok, err := detect.E2E(fixtureRepo(t, "typescript"))
	if err != nil {
		t.Fatalf("E2E: %v", err)
	}
	if !ok || got.Suite != "playwright" {
		t.Errorf("E2E(typescript) = %+v, ok=%v, want suite=playwright, ok=true", got, ok)
	}
}

func TestE2ENoMatch(t *testing.T) {
	_, ok, err := detect.E2E(fixtureRepo(t, "go"))
	if err != nil {
		t.Fatalf("E2E: %v", err)
	}
	if ok {
		t.Error("E2E(go): ok = true, want false")
	}
}

func TestFrontend(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  bool
	}{
		{"tsx extension", []string{"src/App.tsx"}, true},
		{"css extension", []string{"src/styles/App.css"}, true},
		{"components path", []string{"src/components/Button.ts"}, true},
		{"go only", []string{"main.go", "internal/handler.go"}, false},
		{"empty", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detect.Frontend(c.files); got != c.want {
				t.Errorf("Frontend(%v) = %v, want %v", c.files, got, c.want)
			}
		})
	}
}

func TestAppFiles(t *testing.T) {
	changed := []string{"README.md", "docs/guide.md", "LICENSE", "src/main.go", "src/handler.go"}
	got := detect.AppFiles(changed, nil)
	want := []string{"src/handler.go", "src/main.go"}
	if !slices.Equal(got, want) {
		t.Errorf("AppFiles = %v, want %v", got, want)
	}
}

func TestAppFilesClaimedByAddon(t *testing.T) {
	changed := []string{"src/main.go", "infra/main.tf"}
	got := detect.AppFiles(changed, []string{"infra/main.tf"})
	want := []string{"src/main.go"}
	if !slices.Equal(got, want) {
		t.Errorf("AppFiles = %v, want %v", got, want)
	}
}

func TestAppFilesEmpty(t *testing.T) {
	got := detect.AppFiles(nil, nil)
	if len(got) != 0 {
		t.Errorf("AppFiles(nil, nil) = %v, want empty", got)
	}
}
