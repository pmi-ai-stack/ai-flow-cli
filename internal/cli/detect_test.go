package cli

import (
	"encoding/json"
	"testing"
)

func TestDetectLanguages(t *testing.T) {
	home := t.TempDir()
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{
		{"go-svc", fixtureRepoPath(t, "go")},
		{"web", fixtureRepoPath(t, "typescript")},
		{"api", fixtureRepoPath(t, "python")},
	}, nil)

	stdout, _, exitCode := runRoot(t, "detect", "languages", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result detectLanguagesResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(env.Result.Repositories) != 3 {
		t.Fatalf("Repositories = %+v, want 3 entries", env.Result.Repositories)
	}
	if len(env.Result.Repositories[0].Languages) != 1 || env.Result.Repositories[0].Languages[0] != "go" {
		t.Errorf("go-svc languages = %v, want [go]", env.Result.Repositories[0].Languages)
	}
}

func TestDetectRunners(t *testing.T) {
	home := t.TempDir()
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{
		{"go-svc", fixtureRepoPath(t, "go")},
	}, nil)

	stdout, _, exitCode := runRoot(t, "detect", "runners", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result detectRunnersResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.Repositories) != 1 || !env.Result.Repositories[0].Detected || env.Result.Repositories[0].Runner != "go" {
		t.Errorf("Repositories = %+v", env.Result.Repositories)
	}
}

func TestDetectRunnersNoMatch(t *testing.T) {
	home := t.TempDir()
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{
		{"empty", t.TempDir()},
	}, nil)

	stdout, _, exitCode := runRoot(t, "detect", "runners", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result detectRunnersResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.Repositories) != 1 || env.Result.Repositories[0].Detected {
		t.Errorf("Repositories = %+v, want one undetected entry", env.Result.Repositories)
	}
}

func TestDetectMutation(t *testing.T) {
	home := t.TempDir()
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{
		{"go-svc", fixtureRepoPath(t, "go")},
	}, nil)

	stdout, _, exitCode := runRoot(t, "detect", "mutation", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result detectMutationResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.Repositories) != 1 || env.Result.Repositories[0].Tool != "gremlins" {
		t.Errorf("Repositories = %+v", env.Result.Repositories)
	}
}

func TestDetectE2E(t *testing.T) {
	home := t.TempDir()
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{
		{"web", fixtureRepoPath(t, "typescript")},
	}, nil)

	stdout, _, exitCode := runRoot(t, "detect", "e2e", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result detectE2EResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.Repositories) != 1 || env.Result.Repositories[0].Suite != "playwright" {
		t.Errorf("Repositories = %+v", env.Result.Repositories)
	}
}

func TestDetectFrontend(t *testing.T) {
	home := t.TempDir()
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"web", fixtureRepoPath(t, "typescript")}}, []string{"src/App.tsx"})

	stdout, _, exitCode := runRoot(t, "detect", "frontend", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result detectFrontendResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.Result.HasFrontendFiles {
		t.Error("HasFrontendFiles = false, want true")
	}
}

func TestDetectFrontendNoMatch(t *testing.T) {
	home := t.TempDir()
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"go-svc", fixtureRepoPath(t, "go")}}, []string{"main.go"})

	stdout, _, exitCode := runRoot(t, "detect", "frontend", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result detectFrontendResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Result.HasFrontendFiles {
		t.Error("HasFrontendFiles = true, want false")
	}
}
