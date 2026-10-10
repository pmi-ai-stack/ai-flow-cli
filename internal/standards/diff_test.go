package standards

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff/handofftest"
)

func testPack(t *testing.T, m Manifest, files fstest.MapFS) *Pack {
	t.Helper()
	fp, err := Fingerprint(files)
	if err != nil {
		t.Fatal(err)
	}
	return &Pack{ID: m.ID, Version: m.Version, Fingerprint: fp, Sections: m.Sections, Checks: m.Checks, Overrides: m.Overrides, FS: files}
}

func basePack() (Manifest, fstest.MapFS) {
	m := Manifest{
		ID: "acme", Version: "1.0.0",
		Sections: []Section{
			{ID: "go.errors", Path: "languages/go/guidance/errors.md", Severity: "must"},
			{ID: "go.naming", Path: "languages/go/guidance/naming.md", Severity: "should"},
		},
		Checks: []Check{
			{ID: "go.lint", Kind: "lint", Tool: "golangci-lint", Config: "languages/go/checks/golangci.yml", Severity: "should"},
			{ID: "infra.terraform.policy", Kind: "policy", Tool: "conftest", Config: "infra/terraform/checks/policy", Severity: "must",
				AppliesTo: AppliesTo{Addons: []string{"terraform"}}},
		},
	}
	files := fstest.MapFS{
		"standards.json":                            {Data: []byte("{}")},
		"languages/go/guidance/errors.md":           {Data: []byte("wrap errors\n")},
		"languages/go/guidance/naming.md":           {Data: []byte("short names\n")},
		"languages/go/checks/golangci.yml":          {Data: []byte("linters: {}\n")},
		"infra/terraform/checks/policy/naming.rego": {Data: []byte("package main\n")},
	}
	return m, files
}

func clone(m Manifest, files fstest.MapFS) (Manifest, fstest.MapFS) {
	data, _ := json.Marshal(m)
	var c Manifest
	_ = json.Unmarshal(data, &c)
	f := fstest.MapFS{}
	for k, v := range files {
		f[k] = &fstest.MapFile{Data: slices.Clone(v.Data)}
	}
	return c, f
}

func TestDiffIdentical(t *testing.T) {
	m, f := basePack()
	m2, f2 := clone(m, f)
	d, err := Diff(testPack(t, m, f), testPack(t, m2, f2))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Identical {
		t.Error("identical packs: Identical = false")
	}
	if len(d.Checks.Added)+len(d.Checks.Removed)+len(d.Checks.Changed)+len(d.Sections.Changed)+len(d.Files.Changed) != 0 {
		t.Errorf("identical packs reported differences: %+v", d)
	}
}

func TestDiffEntries(t *testing.T) {
	m, f := basePack()
	b, bf := clone(m, f)

	// A should check that now blocks, and a policy that now applies to helm too.
	b.Checks[0].Severity = "must"
	b.Checks[1].AppliesTo.Addons = []string{"helm", "terraform"}
	// A check added and a section removed.
	b.Checks = append(b.Checks, Check{ID: "org.secrets", Kind: "secrets", Tool: "gitleaks", Severity: "must"})
	b.Sections = b.Sections[:1]
	delete(bf, "languages/go/guidance/naming.md")
	// Guidance text changed under an unchanged section entry.
	bf["languages/go/guidance/errors.md"] = &fstest.MapFile{Data: []byte("wrap errors with %w\n")}
	// Rego changed under the policy's config directory.
	bf["infra/terraform/checks/policy/naming.rego"] = &fstest.MapFile{Data: []byte("package main\ndeny[msg] { false }\n")}
	// An override added.
	b.Overrides = []Override{{ID: "go.lint", Reason: "stricter config"}}

	d, err := Diff(testPack(t, m, f), testPack(t, b, bf))
	if err != nil {
		t.Fatal(err)
	}
	if d.Identical {
		t.Fatal("Identical = true")
	}
	if !slices.Equal(d.Checks.Added, []string{"org.secrets"}) || len(d.Checks.Removed) != 0 {
		t.Errorf("checks added/removed = %v / %v", d.Checks.Added, d.Checks.Removed)
	}
	if !slices.Equal(d.Sections.Removed, []string{"go.naming"}) || len(d.Sections.Added) != 0 {
		t.Errorf("sections added/removed = %v / %v", d.Sections.Added, d.Sections.Removed)
	}
	if !slices.Equal(d.Overrides.Added, []string{"go.lint"}) {
		t.Errorf("overrides added = %v", d.Overrides.Added)
	}

	if len(d.Checks.Changed) != 2 {
		t.Fatalf("checks changed = %+v, want go.lint and infra.terraform.policy", d.Checks.Changed)
	}
	lint, policy := d.Checks.Changed[0], d.Checks.Changed[1]
	if lint.ID != "go.lint" || !slices.Equal(lint.Changes, []FieldChange{{Field: "severity", From: "should", To: "must"}}) || lint.ConfigChanged {
		t.Errorf("go.lint change = %+v", lint)
	}
	if policy.ID != "infra.terraform.policy" || !policy.ConfigChanged ||
		!slices.Equal(policy.Changes, []FieldChange{{Field: "applies_to", From: `{"addons":["terraform"]}`, To: `{"addons":["helm","terraform"]}`}}) {
		t.Errorf("policy change = %+v", policy)
	}

	if len(d.Sections.Changed) != 1 || d.Sections.Changed[0].ID != "go.errors" ||
		!d.Sections.Changed[0].ContentChanged || len(d.Sections.Changed[0].Changes) != 0 {
		t.Errorf("sections changed = %+v, want go.errors with content only", d.Sections.Changed)
	}

	if !slices.Equal(d.Files.Changed, []string{"infra/terraform/checks/policy/naming.rego", "languages/go/guidance/errors.md"}) ||
		!slices.Equal(d.Files.Removed, []string{"languages/go/guidance/naming.md"}) || len(d.Files.Added) != 0 {
		t.Errorf("files = %+v", d.Files)
	}
}

// TestDiffIsByteStable: manifest order never changes the result.
func TestDiffIsByteStable(t *testing.T) {
	m, f := basePack()
	b, bf := clone(m, f)
	b.Checks[0].Severity = "must"
	b.Checks = append(b.Checks, Check{ID: "aaa.first", Kind: "lint", Severity: "may"}, Check{ID: "zzz.last", Kind: "lint", Severity: "may"})
	calls := 0
	handofftest.RequireByteStable(t, func() ([]byte, error) {
		bb, bbf := clone(b, bf)
		if calls++; calls%2 == 0 {
			slices.Reverse(bb.Checks)
			slices.Reverse(bb.Sections)
		}
		d, err := Diff(testPack(t, m, f), testPack(t, bb, bbf))
		if err != nil {
			return nil, err
		}
		return json.Marshal(d)
	})
}

// TestFingerprintUnchangedByFileDigests pins Fingerprint to its
// original definition (sha256 over the sorted "<path>\x00<hex sha256>\n"
// lines), written out independently here: phase 52 moved the per-file
// walk into fileDigests, and every resolution hash depends on these
// bytes not moving.
func TestFingerprintUnchangedByFileDigests(t *testing.T) {
	_, files := basePack()
	var lines []string
	for path, f := range files {
		sum := sha256.Sum256(f.Data)
		lines = append(lines, path+"\x00"+hex.EncodeToString(sum[:])+"\n")
	}
	slices.Sort(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
	}
	want := "sha256:" + hex.EncodeToString(h.Sum(nil))

	got, err := Fingerprint(files)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Fingerprint = %s, want %s", got, want)
	}
}
