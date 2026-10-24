package cli

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// fileURL is dir as a file URL. On Windows dir is C:\..., and
// "file://"+dir does not parse (the drive colon reads as a port): the
// URL form is file:///C:/..., which go-git's file transport turns back
// into C:/... before running git-upload-pack.
func fileURL(dir string) string {
	return (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(dir), "/")}).String()
}

type diffEnvelope struct {
	OK     bool `json:"ok"`
	Result *struct {
		A         standards.PackIdentity `json:"a"`
		B         standards.PackIdentity `json:"b"`
		Identical bool                   `json:"identical"`
		Checks    standards.EntryDiff    `json:"checks"`
		Sections  standards.EntryDiff    `json:"sections"`
		Files     standards.FileDiff     `json:"files"`
	} `json:"result"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func runDiff(t *testing.T, wantExit int, args ...string) diffEnvelope {
	t.Helper()
	stdout, _, ec := runRoot(t, append([]string{"standards", "diff"}, args...)...)
	if ec != wantExit {
		t.Fatalf("standards diff %v: exit %d, want %d\n%s", args, ec, wantExit, stdout.String())
	}
	var env diffEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout.String())
	}
	return env
}

func TestStandardsDiffReferenceWithItself(t *testing.T) {
	t.Chdir(t.TempDir()) // no directory named "reference" here
	env := runDiff(t, 0, "reference", "reference")
	if !env.Result.Identical || len(env.Result.Files.Changed) != 0 {
		t.Errorf("result = %+v, want identical", env.Result)
	}
}

// TestStandardsDiffAForkedPack is the operator's question for a pack
// bump: export the reference pack, make it stricter in three ways, and
// diff — exactly those three differences come back.
func TestStandardsDiffAForkedPack(t *testing.T) {
	fork := filepath.Join(t.TempDir(), "fork")
	if _, _, ec := runRoot(t, "standards", "export", fork); ec != 0 {
		t.Fatalf("standards export: exit %d", ec)
	}

	manifestPath := filepath.Join(fork, "standards.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	checks := m["checks"].([]any)
	for _, c := range checks {
		if c.(map[string]any)["id"] == "go.dependencies" {
			c.(map[string]any)["severity"] = "must" // was should
		}
	}
	m["checks"] = append(checks, map[string]any{"id": "org.extra", "kind": "layout", "severity": "may"})
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, out, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fork, "org", "guidance", "naming.md"), []byte("# Naming\n\nNew rules.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(t.TempDir())
	env := runDiff(t, 0, "reference", fork)
	r := env.Result
	if r.Identical {
		t.Fatal("identical = true")
	}
	if r.A.ID != "reference" || r.B.ID != "reference" {
		t.Errorf("ids = %s / %s, want the manifest id on both sides", r.A.ID, r.B.ID)
	}
	if !slices.Equal(r.Checks.Added, []string{"org.extra"}) || len(r.Checks.Removed) != 0 {
		t.Errorf("checks added/removed = %v / %v", r.Checks.Added, r.Checks.Removed)
	}
	if len(r.Checks.Changed) != 1 || r.Checks.Changed[0].ID != "go.dependencies" ||
		!slices.Equal(r.Checks.Changed[0].Changes, []standards.FieldChange{{Field: "severity", From: "should", To: "must"}}) {
		t.Errorf("checks changed = %+v", r.Checks.Changed)
	}
	if len(r.Sections.Changed) != 1 || r.Sections.Changed[0].ID != "org.naming" || !r.Sections.Changed[0].ContentChanged {
		t.Errorf("sections changed = %+v", r.Sections.Changed)
	}
	if !slices.Equal(r.Files.Changed, []string{"org/guidance/naming.md", "standards.json"}) {
		t.Errorf("files changed = %v", r.Files.Changed)
	}
}

func TestStandardsDiffErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	if env := runDiff(t, 2, "reference", "no-such-pack"); env.Error.Code != "usage" {
		t.Errorf("unknown argument: code = %s, want usage", env.Error.Code)
	}
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "standards.json"), []byte(`{"id": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if env := runDiff(t, 6, "reference", broken); env.Error.Code != "pack_invalid" {
		t.Errorf("broken pack: code = %s, want pack_invalid", env.Error.Code)
	}
}

// TestStandardsDiffTwoRefsOfOneSource is the pack bump itself: one git
// source at two tags, each synced into its own temporary home. The
// diff names the section the second tag added and its guidance file,
// and nothing from the sync machinery (no .git).
func TestStandardsDiffTwoRefsOfOneSource(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "fixture", Email: "fixture@example.com", When: time.Unix(1700000000, 0).UTC()}
	commitTag := func(tag, manifest string, files map[string]string) {
		t.Helper()
		writeFileTest(t, filepath.Join(dir, "standards.json"), manifest)
		for name, body := range files {
			writeFileTest(t, filepath.Join(dir, name), body)
		}
		if _, err := wt.Add("."); err != nil {
			t.Fatal(err)
		}
		h, err := wt.Commit(tag, &git.CommitOptions{Author: sig})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CreateTag(tag, h, nil); err != nil {
			t.Fatal(err)
		}
	}
	commitTag("v1", `{"schema_version": 1, "id": "acme", "version": "1.0.0",
  "sections": [{"id": "org.a", "path": "org/a.md", "severity": "should"}], "checks": []}`,
		map[string]string{"org/a.md": "## A\n"})
	commitTag("v2", `{"schema_version": 1, "id": "acme", "version": "2.0.0",
  "sections": [{"id": "org.a", "path": "org/a.md", "severity": "should"}, {"id": "org.b", "path": "org/b.md", "severity": "must"}], "checks": []}`,
		map[string]string{"org/b.md": "## B\n"})

	t.Chdir(t.TempDir())
	url := fileURL(dir)
	env := runDiff(t, 0, url+"@v1", url+"@v2")
	r := env.Result
	if r.A.ID != "acme" || r.A.Version != "1.0.0" || r.B.Version != "2.0.0" {
		t.Errorf("a = %+v, b = %+v, want the manifests' own id and versions", r.A, r.B)
	}
	if !slices.Equal(r.Sections.Added, []string{"org.b"}) || len(r.Sections.Changed) != 0 {
		t.Errorf("sections = %+v", r.Sections)
	}
	if !slices.Equal(r.Files.Added, []string{"org/b.md"}) || !slices.Equal(r.Files.Changed, []string{"standards.json"}) || len(r.Files.Removed) != 0 {
		t.Errorf("files = %+v", r.Files)
	}
}
