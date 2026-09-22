package routing

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

func TestProjectKeyFromTicketKey(t *testing.T) {
	cases := map[string]string{
		"PROJ-123":  "PROJ",
		"PLAT-1":    "PLAT",
		"AB12-9999": "AB12",
	}
	for key, want := range cases {
		if got := ProjectKeyFromTicketKey(key); got != want {
			t.Errorf("ProjectKeyFromTicketKey(%q) = %q, want %q", key, got, want)
		}
	}
}

// testRouting builds a config.Routing over a temp local_base with a
// real AGENTS.md written for every repo named in repos, so
// buildResult's instructions-file check always succeeds unless a test
// deliberately omits one.
func testRouting(t *testing.T, repos ...string) (config.Routing, string) {
	t.Helper()
	base := t.TempDir()
	for _, r := range repos {
		dir := filepath.Join(base, r)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# rules\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	cfg := config.Routing{
		ProjectKeyMap: map[string]config.ProjectKeyMapEntry{
			"PROJ": {Team: "Team Beta", Repos: []string{"acme.billing.api", "acme.billing.web"}},
			"PLAT": {Team: "Platform", Repos: []string{"acme.infra", "acme.k8s-config", "acme.ci"}},
		},
		KeywordToRepo: map[string][]string{
			"billing": {"acme.billing.api", "acme.billing.web"},
			"cluster": {"acme.k8s-config"},
		},
		Teams: map[string]config.Team{
			"Team Beta": {LocalBase: base},
			"Platform":  {LocalBase: base},
		},
	}
	return cfg, base
}

func TestLocateNoProjectKeyMatchIsAmbiguousWithEveryRepo(t *testing.T) {
	cfg, _ := testRouting(t, "acme.billing.api", "acme.billing.web", "acme.infra", "acme.k8s-config", "acme.ci")
	_, err := Locate(cfg, "ZZZ-1", "some ticket text")
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "routing_ambiguous" {
		t.Fatalf("err = %v, want routing_ambiguous", err)
	}
	want := []string{"acme.billing.api", "acme.billing.web", "acme.ci", "acme.infra", "acme.k8s-config"}
	if len(envErr.Details) != len(want) {
		t.Fatalf("Details = %v, want %v", envErr.Details, want)
	}
	for i, w := range want {
		if envErr.Details[i] != w {
			t.Errorf("Details[%d] = %q, want %q", i, envErr.Details[i], w)
		}
	}
}

func TestLocateKeywordNarrowsToOne(t *testing.T) {
	cfg, base := testRouting(t, "acme.infra", "acme.k8s-config", "acme.ci")
	res, err := Locate(cfg, "PLAT-1", "please fix the cluster config drift")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if res.Team != "Platform" {
		t.Errorf("Team = %q, want Platform", res.Team)
	}
	if len(res.Repos) != 1 || res.Repos[0].Name != "acme.k8s-config" {
		t.Fatalf("Repos = %+v, want exactly acme.k8s-config", res.Repos)
	}
	if res.Repos[0].LocalPath != filepath.Join(base, "acme.k8s-config") {
		t.Errorf("LocalPath = %q", res.Repos[0].LocalPath)
	}
	if res.Repos[0].InstructionsFile != "AGENTS.md" {
		t.Errorf("InstructionsFile = %q, want AGENTS.md", res.Repos[0].InstructionsFile)
	}
}

func TestLocateEmptyNarrowingKeepsAllCandidatesThenAmbiguousWhenTicketNamesNone(t *testing.T) {
	cfg, _ := testRouting(t, "acme.billing.api", "acme.billing.web")
	_, err := Locate(cfg, "PROJ-1", "a ticket with no keyword and no repo name in it")
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "routing_ambiguous" {
		t.Fatalf("err = %v, want routing_ambiguous", err)
	}
	if len(envErr.Details) != 2 {
		t.Fatalf("Details = %v, want both billing repos", envErr.Details)
	}
}

func TestLocateTicketNamesOneOfSeveralNarrowedRepos(t *testing.T) {
	cfg, _ := testRouting(t, "acme.billing.api", "acme.billing.web")
	res, err := Locate(cfg, "PROJ-1", "fix a bug in acme.billing.api's retry logic")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if len(res.Repos) != 1 || res.Repos[0].Name != "acme.billing.api" {
		t.Fatalf("Repos = %+v, want exactly acme.billing.api", res.Repos)
	}
}

func TestLocateTicketNamesBothOfSeveralNarrowedRepos(t *testing.T) {
	cfg, _ := testRouting(t, "acme.billing.api", "acme.billing.web")
	res, err := Locate(cfg, "PROJ-1", "billing change touching acme.billing.api and acme.billing.web")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if len(res.Repos) != 2 {
		t.Fatalf("Repos = %+v, want both billing repos", res.Repos)
	}
}

func TestLocateMissingRepoInstructions(t *testing.T) {
	cfg, base := testRouting(t, "acme.billing.api")
	// Remove the AGENTS.md testRouting wrote, leave the directory empty.
	if err := os.Remove(filepath.Join(base, "acme.billing.api", "AGENTS.md")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err := Locate(cfg, "PROJ-1", "acme.billing.api needs a fix")
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "missing_repo_instructions" {
		t.Fatalf("err = %v, want missing_repo_instructions", err)
	}
}

func TestLocateFallsBackToClaudeMd(t *testing.T) {
	cfg, base := testRouting(t, "acme.billing.api")
	agentsPath := filepath.Join(base, "acme.billing.api", "AGENTS.md")
	if err := os.Remove(agentsPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "acme.billing.api", "CLAUDE.md"), []byte("@AGENTS.md\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	res, err := Locate(cfg, "PROJ-1", "acme.billing.api needs a fix")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if res.Repos[0].InstructionsFile != "CLAUDE.md" {
		t.Errorf("InstructionsFile = %q, want CLAUDE.md", res.Repos[0].InstructionsFile)
	}
}

func TestConfirmDerivesTeamFromRepoNames(t *testing.T) {
	cfg, _ := testRouting(t, "acme.infra", "acme.k8s-config")
	res, err := Confirm(cfg, []string{"acme.k8s-config"})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if res.Team != "Platform" {
		t.Errorf("Team = %q, want Platform", res.Team)
	}
}

func TestConfirmRejectsUnknownRepo(t *testing.T) {
	cfg, _ := testRouting(t)
	_, err := Confirm(cfg, []string{"not-a-real-repo"})
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "usage" {
		t.Fatalf("err = %v, want usage", err)
	}
}

func TestConfirmRejectsMixedTeams(t *testing.T) {
	cfg, _ := testRouting(t, "acme.billing.api", "acme.infra")
	_, err := Confirm(cfg, []string{"acme.billing.api", "acme.infra"})
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "usage" {
		t.Fatalf("err = %v, want usage", err)
	}
}

func TestConfirmRequiresAtLeastOneRepo(t *testing.T) {
	cfg, _ := testRouting(t)
	_, err := Confirm(cfg, nil)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "usage" {
		t.Fatalf("err = %v, want usage", err)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir available: %v", err)
	}
	got, err := expandHome("~/projects")
	if err != nil {
		t.Fatalf("expandHome: %v", err)
	}
	if want := filepath.Join(home, "projects"); got != want {
		t.Errorf("expandHome(~/projects) = %q, want %q", got, want)
	}
	if got, _ := expandHome("/already/absolute"); got != "/already/absolute" {
		t.Errorf("expandHome should leave a non-~ path unchanged, got %q", got)
	}
}
