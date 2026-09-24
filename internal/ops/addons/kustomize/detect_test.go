package kustomize

import (
	"sort"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
)

func TestDetectFindsBaseAndOverlayAsIndependentTargets(t *testing.T) {
	a := New(nil, nil)
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	for _, tg := range targets {
		paths = append(paths, tg.Path)
		if tg.Addon != "kustomize" {
			t.Errorf("target %q has addon %q, want kustomize", tg.Path, tg.Addon)
		}
	}
	sort.Strings(paths)
	want := []string{"base", "nested/outer", "overlay"}
	if len(paths) != len(want) {
		t.Fatalf("Detect paths = %v, want %v", paths, want)
	}
	for i, w := range want {
		if paths[i] != w {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], w)
		}
	}
}

func TestDetectDoesNotDescendIntoAFoundOverlay(t *testing.T) {
	a := New(nil, nil)
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata/nested"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Path != "outer" {
		t.Fatalf("Detect = %v, want exactly [outer] — outer/inner must never be scanned once outer itself is found", targets)
	}
}

func TestDetectFiltersFilesToChangedUnderEachTarget(t *testing.T) {
	a := New(nil, nil)
	changed := []string{"base/deployment.yaml", "overlay/kustomization.yaml", "unrelated/other.go"}
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata"}, changed)
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string][]string{}
	for _, tg := range targets {
		byPath[tg.Path] = tg.Files
	}
	if got := byPath["base"]; len(got) != 1 || got[0] != "base/deployment.yaml" {
		t.Errorf("base target Files = %v, want [base/deployment.yaml]", got)
	}
	if got := byPath["overlay"]; len(got) != 1 || got[0] != "overlay/kustomization.yaml" {
		t.Errorf("overlay target Files = %v, want [overlay/kustomization.yaml]", got)
	}
}

func TestDetectClaimsNestedChangedFilesTooViaPrefixMatch(t *testing.T) {
	a := New(nil, nil)
	changed := []string{"overlay/patches/deployment-patch.yaml"}
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata"}, changed)
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range targets {
		if tg.Path == "overlay" {
			if len(tg.Files) != 1 || tg.Files[0] != "overlay/patches/deployment-patch.yaml" {
				t.Fatalf("overlay target Files = %v, want the nested patch file claimed via prefix match", tg.Files)
			}
			return
		}
	}
	t.Fatal("no overlay target found")
}
