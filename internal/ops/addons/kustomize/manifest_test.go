package kustomize

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestsFindsEveryDocument(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rendered.yaml", ""+
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\n---\n"+
		"apiVersion: v1\nkind: Service\nmetadata:\n  name: web\n  namespace: default\n")

	resources, err := parseManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 {
		t.Fatalf("parseManifests = %d resources, want 2", len(resources))
	}
	want := []string{"default/Deployment/web", "default/Service/web"}
	for i, w := range want {
		if resources[i].address() != w {
			t.Errorf("resources[%d].address() = %q, want %q (sorted order)", i, resources[i].address(), w)
		}
	}
}

func TestParseManifestsSkipsDocumentMissingApiVersionOrKind(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "partial.yaml", "kind: Deployment\nmetadata:\n  name: no-api-version\n")
	writeFile(t, dir, "blank.yaml", "---\n---\n")

	resources, err := parseManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Fatalf("parseManifests = %v, want none", resources)
	}
}

func TestParseManifestsToleratesMalformedYAMLWithoutFailing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "broken.yaml", "apiVersion: v1\nkind: [unterminated\n")
	writeFile(t, dir, "good.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: ok\n")

	resources, err := parseManifests(dir)
	if err != nil {
		t.Fatalf("parseManifests returned an error for a malformed file, want tolerant skip: %v", err)
	}
	if len(resources) != 1 || resources[0].name != "ok" {
		t.Fatalf("parseManifests = %v, want exactly the one well-formed resource", resources)
	}
}

func TestResourceAddressClusterScoped(t *testing.T) {
	r := resource{kind: "Namespace", name: "prod"}
	if got, want := r.address(), "-/Namespace/prod"; got != want {
		t.Errorf("address() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
