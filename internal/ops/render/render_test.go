package render_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/testdata/fixtureaddon"
)

func writeSource(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func target(repo, path string) ops.Target {
	return ops.Target{Addon: "fixture", Repo: repo, Path: path}
}

// TestRenderRunsTheToolOnceAcrossTwoTickets is the plan's own "Done
// when": rendering the same tree twice — once per ticket, both sharing
// identical source content — runs the addon's Render exactly once, and
// both tickets get their own correct redacted copy.
func TestRenderRunsTheToolOnceAcrossTwoTickets(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	writeSource(t, src, map[string]string{"main.tf": "resource \"x\" {}\n"})

	addon := fixtureaddon.New("fixture", "infra")
	tgt := target("acme.infra", "infra")

	res1, err := render.Render(context.Background(), render.Options{
		Addon: addon, Target: tgt, Ref: ops.RefBranch, SourceDir: src, Home: home, Key: "PROJ-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := render.Render(context.Background(), render.Options{
		Addon: addon, Target: tgt, Ref: ops.RefBranch, SourceDir: src, Home: home, Key: "PROJ-2",
	})
	if err != nil {
		t.Fatal(err)
	}

	if addon.RenderCalls != 1 {
		t.Errorf("RenderCalls = %d, want 1 (second ticket must reuse the cache)", addon.RenderCalls)
	}
	if res1.Hash != res2.Hash {
		t.Errorf("hash mismatch across tickets: %q vs %q", res1.Hash, res2.Hash)
	}

	data1, err := os.ReadFile(filepath.Join(home, "PROJ-1", res1.RedactedDir, "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data1) != "resource \"x\" {}\n" {
		t.Errorf("PROJ-1 redacted copy = %q, want the source content unchanged (nothing sensitive)", data1)
	}
	data2, err := os.ReadFile(filepath.Join(home, "PROJ-2", res2.RedactedDir, "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != "resource \"x\" {}\n" {
		t.Errorf("PROJ-2 redacted copy = %q, want the source content unchanged", data2)
	}
}

// TestRenderRedactsMarkedSecretButNotTheRealCache proves the real/
// redacted split at the file-content level: the tool's own marking
// survives, unredacted, in the real cache (only readable by this
// package/Exec), and is replaced by its <redacted:...> token in the
// ticket's own redacted copy.
func TestRenderRedactsMarkedSecretButNotTheRealCache(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	const secret = "super-secret-value"
	writeSource(t, src, map[string]string{
		"secret.txt": secret,
		"plain.txt":  "nothing sensitive here",
	})

	addon := fixtureaddon.New("fixture", "infra")
	addon.SensitiveFiles = []string{"secret.txt"}
	tgt := target("acme.infra", "infra")

	res, err := render.Render(context.Background(), render.Options{
		Addon: addon, Target: tgt, Ref: ops.RefBranch, SourceDir: src, Home: home, Key: "PROJ-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	realData, err := os.ReadFile(filepath.Join(render.CachePath(home, res.Hash), "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(realData) != secret {
		t.Errorf("real cache secret.txt = %q, want the unredacted value (only Exec/this package reads it)", realData)
	}

	redactedData, err := os.ReadFile(filepath.Join(home, "PROJ-1", res.RedactedDir, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(redactedData), secret) {
		t.Errorf("redacted copy secret.txt = %q, must not contain the raw secret", redactedData)
	}
	if !strings.HasPrefix(string(redactedData), "<redacted:") {
		t.Errorf("redacted copy secret.txt = %q, want a <redacted:...> token", redactedData)
	}

	plainData, err := os.ReadFile(filepath.Join(home, "PROJ-1", res.RedactedDir, "plain.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(plainData) != "nothing sensitive here" {
		t.Errorf("redacted copy plain.txt = %q, want unchanged (nothing marked)", plainData)
	}
}

// TestRenderRealCacheIsMode0700 asserts the spec's own mode, verbatim.
func TestRenderRealCacheIsMode0700(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	writeSource(t, src, map[string]string{"a.txt": "a"})

	addon := fixtureaddon.New("fixture", "infra")
	res, err := render.Render(context.Background(), render.Options{
		Addon: addon, Target: target("acme.infra", "infra"), Ref: ops.RefBranch, SourceDir: src, Home: home, Key: "PROJ-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(render.CachePath(home, res.Hash))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("real cache dir mode = %o, want 0700", perm)
	}
}

// TestNoArtifactNamesTheCachePath is the plan's own grep test: nothing
// this package ever writes under a ticket directory contains the real
// cache path as a substring.
func TestNoArtifactNamesTheCachePath(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	writeSource(t, src, map[string]string{"a.txt": "hello"})

	addon := fixtureaddon.New("fixture", "infra")
	res, err := render.Render(context.Background(), render.Options{
		Addon: addon, Target: target("acme.infra", "infra"), Ref: ops.RefBranch, SourceDir: src, Home: home, Key: "PROJ-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	cachePath := render.CachePath(home, res.Hash)
	ticketDir := filepath.Join(home, "PROJ-1")
	err = filepath.WalkDir(ticketDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), cachePath) {
			t.Errorf("%s contains the real cache path %q", path, cachePath)
		}
		if strings.Contains(string(data), filepath.Join(home, "cache")) {
			t.Errorf("%s names the cache directory", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRenderOfflineRefUsesResolvedSourceNotRelativePath proves the
// call-scoped Target passed to Addon.Render carries the resolved
// absolute SourceDir, not the canonical repo-relative Path — the
// design decision the plan records for the frozen Addon interface.
func TestRenderPassesResolvedSourceDirToAddon(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	writeSource(t, src, map[string]string{"only.txt": "x"})

	addon := fixtureaddon.New("fixture", "infra")
	// The canonical Target.Path ("infra", relative) does not exist on
	// disk here at all — only SourceDir does. If render.Render passed
	// the canonical Path through unchanged, Addon.Render's os.ReadDir
	// would fail.
	_, err := render.Render(context.Background(), render.Options{
		Addon: addon, Target: target("acme.infra", "infra"), Ref: ops.RefBranch, SourceDir: src, Home: home, Key: "PROJ-1",
	})
	if err != nil {
		t.Fatalf("Render: %v (addon should have received SourceDir, not the canonical relative Path)", err)
	}
}

// TestRedactedDirMatchesWhatRenderItselfWrote is phase 25's own first
// caller of RedactedDir: given a hash a prior Render call already
// produced, RedactedDir must resolve to the exact same directory
// Render's own Result.RedactedDir named — the property the
// "policy"/"scan" check dispatch (internal/cli/checks.go) depends on
// to locate a target's rendering without rendering it again.
func TestRedactedDirMatchesWhatRenderItselfWrote(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	writeSource(t, src, map[string]string{"main.tf": "resource \"x\" {}\n"})

	addon := fixtureaddon.New("fixture", "infra")
	tgt := target("acme.infra", "infra")
	res, err := render.Render(context.Background(), render.Options{
		Addon: addon, Target: tgt, Ref: ops.RefBranch, SourceDir: src, Home: home, Key: "PROJ-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	got := render.RedactedDir(filepath.Join(home, "PROJ-1"), tgt, res.Hash)
	want := filepath.Join(home, "PROJ-1", res.RedactedDir)
	if got != want {
		t.Errorf("RedactedDir = %q, want %q (Render's own result)", got, want)
	}
}

// TestLoadReturnsTheRealRenderingAndItsMarkings is phase 20's own first
// caller of Load: given a hash Render already produced, Load returns
// the *ops.Rendered (real, unredacted dir + markings) Addon.Validate/
// Drift/DryRun need — not the redacted copy.
func TestLoadReturnsTheRealRenderingAndItsMarkings(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	const secret = "super-secret-value"
	writeSource(t, src, map[string]string{"secret.txt": secret})

	addon := fixtureaddon.New("fixture", "infra")
	addon.SensitiveFiles = []string{"secret.txt"}
	res, err := render.Render(context.Background(), render.Options{
		Addon: addon, Target: target("acme.infra", "infra"), Ref: ops.RefBranch, SourceDir: src, Home: home, Key: "PROJ-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := render.Load(home, res.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Dir != render.CachePath(home, res.Hash) {
		t.Errorf("Load Dir = %q, want %q", rendered.Dir, render.CachePath(home, res.Hash))
	}
	data, err := os.ReadFile(filepath.Join(rendered.Dir, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != secret {
		t.Errorf("real cache content = %q, want the unredacted secret (Load reads the real rendering, not the redacted copy)", data)
	}
	if len(rendered.Marked) != 1 || rendered.Marked[0].Value != secret {
		t.Errorf("Load Marked = %+v, want one entry carrying the secret value", rendered.Marked)
	}
}

// TestLoadOfAnUnrenderedHashHasNoMarkings proves Load does not error on
// a hash nothing has rendered yet — it simply carries no markings, the
// same tolerant-of-absence shape readMarkings/ReadArtifact already use.
func TestLoadOfAnUnrenderedHashHasNoMarkings(t *testing.T) {
	home := t.TempDir()
	rendered, err := render.Load(home, "0000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.Marked) != 0 {
		t.Errorf("Marked = %+v, want none", rendered.Marked)
	}
}
