package helm

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
)

func TestDetectFindsChartDirectory(t *testing.T) {
	a := New(nil, nil)
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Path != "chart" || targets[0].Addon != "helm" {
		t.Fatalf("Detect = %v, want exactly one target at \"chart\"", targets)
	}
}

func TestDetectDoesNotDescendIntoAFoundChart(t *testing.T) {
	a := New(nil, nil)
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata/chart"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Path != "." {
		t.Fatalf("Detect = %v, want exactly one target at \".\" — templates/ must never be scanned as a nested chart", targets)
	}
}

func TestDetectClaimsNestedChangedFilesViaPrefixMatch(t *testing.T) {
	a := New(nil, nil)
	changed := []string{"chart/templates/deployment.yaml", "unrelated/other.go"}
	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata"}, changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets[0].Files) != 1 || targets[0].Files[0] != "chart/templates/deployment.yaml" {
		t.Fatalf("target Files = %v, want the nested template file claimed via prefix match", targets[0].Files)
	}
}
