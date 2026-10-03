package kubectl

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
)

func TestDetectFindsEveryManifestsDirectoryIncludingNested(t *testing.T) {
	a := New(nil, nil)
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata/manifests"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	for _, tg := range targets {
		paths = append(paths, tg.Path)
		if tg.Addon != "kubectl" {
			t.Errorf("target %q has addon %q, want kubectl", tg.Path, tg.Addon)
		}
	}
	sort.Strings(paths)
	want := []string{".", "nested"}
	if len(paths) != len(want) || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("Detect paths = %v, want %v (the top-level directory itself and its own nested subdirectory, as two independent targets)", paths, want)
	}
}

func TestDetectSkipsChartOwnedDirectoriesEntirely(t *testing.T) {
	a := New(nil, nil)
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata/notmanifests"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("Detect = %v, want none — testdata/notmanifests/chart/ carries Chart.yaml and must be excluded, itself and its templates/ subdirectory", targets)
	}
}

func TestDetectFiltersFilesToChangedUnderEachTarget(t *testing.T) {
	a := New(nil, nil)
	changed := []string{"deployment.yaml", "nested/job.yaml", "unrelated/other.go"}
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata/manifests"}, changed)
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string][]string{}
	for _, tg := range targets {
		byPath[tg.Path] = tg.Files
	}
	if got := byPath["."]; len(got) != 1 || got[0] != "deployment.yaml" {
		t.Errorf("top-level target Files = %v, want [deployment.yaml]", got)
	}
	if got := byPath["nested"]; len(got) != 1 || got[0] != "nested/job.yaml" {
		t.Errorf("nested target Files = %v, want [nested/job.yaml]", got)
	}
}

// TestDetectByteScanKeepsEveryManifestShape pins dirHasManifest's two
// shortcuts as result-preserving: the apiVersion byte scan covers the
// whole file (a manifest after a long header, or as a later document,
// is still found), matches a quoted key, and a directory of large
// non-manifest YAML still yields no target.
func TestDetectByteScanKeepsEveryManifestShape(t *testing.T) {
	root := t.TempDir()
	header := strings.Repeat("# licence text that is long enough to push the manifest well past any prefix\n", 200)
	files := map[string]string{
		"afterheader/deploy.yaml": header + "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: a\n",
		"seconddoc/multi.yaml":    "foo: bar\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: b\n",
		"quoted/cm.yml":           "\"apiVersion\": \"v1\"\n\"kind\": \"ConfigMap\"\n\"metadata\": {\"name\": \"c\"}\n",
		"config/values.yaml":      strings.Repeat("key: value\nlist: [1, 2, 3]\n", 2000),
		"mentions/notes.yaml":     "text: the word apiVersion appears here but no manifest does\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	targets, err := New(nil, nil).Detect(detect.Repo{Name: "repo", LocalPath: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, tg := range targets {
		paths = append(paths, tg.Path)
	}
	want := []string{"afterheader", "quoted", "seconddoc"}
	if !slices.Equal(paths, want) {
		t.Fatalf("Detect paths = %v, want %v", paths, want)
	}
}
