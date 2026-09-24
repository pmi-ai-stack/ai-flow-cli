package kubectl

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

func TestRenderCopiesFilesAndMarksSecretValues(t *testing.T) {
	a := New(nil, nil)
	out := t.TempDir()

	rendered, err := a.Render(context.Background(), ops.Target{Path: filepath.Join("testdata", "manifests")}, ops.RefBranch, out)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(out, "secret.yaml")); err != nil {
		t.Errorf("Render did not copy secret.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "nested", "job.yaml")); err != nil {
		t.Errorf("Render did not recursively copy nested/job.yaml: %v", err)
	}

	var values []string
	for _, m := range rendered.Marked {
		values = append(values, m.Value)
	}
	sort.Strings(values)
	want := []string{"czNjcjN0LXZhbHVl", "plain-secret-token"}
	if len(values) != len(want) || values[0] != want[0] || values[1] != want[1] {
		t.Fatalf("Rendered.Marked values = %v, want %v (the Secret's data.password and stringData.token, verbatim)", values, want)
	}
}

func TestRenderMarksNothingWhenNoSecretPresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "configmap.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\ndata:\n  key: value\n")

	a := New(nil, nil)
	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Path: dir}, ops.RefBranch, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.Marked) != 0 {
		t.Errorf("Rendered.Marked = %v, want none — a ConfigMap is never marked, only a Secret", rendered.Marked)
	}
}

func TestRenderToleratesMalformedFileRatherThanFailing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "broken.yaml", "apiVersion: v1\nkind: [unterminated\n")

	a := New(nil, nil)
	out := t.TempDir()
	if _, err := a.Render(context.Background(), ops.Target{Path: dir}, ops.RefBranch, out); err != nil {
		t.Fatalf("Render failed on a malformed manifest, want a tolerant copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "broken.yaml")); err != nil {
		t.Errorf("Render did not still copy the malformed file: %v", err)
	}
}
