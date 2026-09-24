package kubectl

import (
	"sort"
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
