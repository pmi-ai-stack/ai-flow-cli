package standards

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// PackDiff is what bumping a pack from A to B changes: manifest entries
// added, removed or changed (by id), and every file of the two trees
// by content digest. Every list is sorted, so the same two packs always
// produce the same value.
type PackDiff struct {
	A         PackIdentity `json:"a"`
	B         PackIdentity `json:"b"`
	Identical bool         `json:"identical"`
	Sections  EntryDiff    `json:"sections"`
	Checks    EntryDiff    `json:"checks"`
	Overrides EntryDiff    `json:"overrides"`
	Files     FileDiff     `json:"files"`
}

// PackIdentity names one side of a diff.
type PackIdentity struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

// EntryDiff is one manifest list's difference, matched by id.
type EntryDiff struct {
	Added   []string       `json:"added"`
	Removed []string       `json:"removed"`
	Changed []EntryChanged `json:"changed"`
}

// EntryChanged is one id present on both sides that differs: in its
// manifest fields (Changes), in the guidance file a section points at
// (ContentChanged), or in any file at or under a check's config path
// (ConfigChanged). An entry that changed only in content has no
// Changes.
type EntryChanged struct {
	ID             string        `json:"id"`
	Changes        []FieldChange `json:"changes"`
	ContentChanged bool          `json:"content_changed,omitempty"`
	ConfigChanged  bool          `json:"config_changed,omitempty"`
}

// FieldChange is one manifest field's old and new value. applies_to is
// rendered as canonical JSON (sorted lists), so a scope change reads
// exactly.
type FieldChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// FileDiff is every file path of the two trees that was added, removed
// or whose content differs.
type FileDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

// Diff compares two loaded packs. It reads both trees in full (every
// file's digest), so it answers for content that a manifest-only
// comparison would miss: guidance text, linter configs, rego.
func Diff(a, b *Pack) (PackDiff, error) {
	da, err := fileDigests(a.FS)
	if err != nil {
		return PackDiff{}, fmt.Errorf("standards: diff: read pack %s: %w", a.ID, err)
	}
	db, err := fileDigests(b.FS)
	if err != nil {
		return PackDiff{}, fmt.Errorf("standards: diff: read pack %s: %w", b.ID, err)
	}
	files := diffFiles(da, db)
	changedFile := func(path string) bool {
		_, inA := da[path]
		_, inB := db[path]
		return (inA || inB) && da[path] != db[path]
	}
	changedUnder := func(root string) bool {
		if root == "" {
			return false
		}
		for _, p := range slices.Concat(files.Added, files.Removed, files.Changed) {
			if p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/") {
				return true
			}
		}
		return false
	}

	sections := diffEntries(a.Sections, b.Sections,
		func(s Section) string { return s.ID },
		func(x, y Section) []FieldChange {
			return fieldChanges(
				[3]string{"path", x.Path, y.Path},
				[3]string{"severity", x.Severity, y.Severity},
				[3]string{"applies_to", canonicalAppliesTo(x.AppliesTo), canonicalAppliesTo(y.AppliesTo)},
			)
		},
		func(x, y Section, e *EntryChanged) { e.ContentChanged = changedFile(x.Path) || changedFile(y.Path) })

	checks := diffEntries(a.Checks, b.Checks,
		func(c Check) string { return c.ID },
		func(x, y Check) []FieldChange {
			return fieldChanges(
				[3]string{"kind", x.Kind, y.Kind},
				[3]string{"tool", x.Tool, y.Tool},
				[3]string{"config", x.Config, y.Config},
				[3]string{"severity", x.Severity, y.Severity},
				[3]string{"input", x.Input, y.Input},
				[3]string{"applies_to", canonicalAppliesTo(x.AppliesTo), canonicalAppliesTo(y.AppliesTo)},
			)
		},
		func(x, y Check, e *EntryChanged) { e.ConfigChanged = changedUnder(x.Config) || changedUnder(y.Config) })

	overrides := diffEntries(a.Overrides, b.Overrides,
		func(o Override) string { return o.ID },
		func(x, y Override) []FieldChange { return fieldChanges([3]string{"reason", x.Reason, y.Reason}) },
		func(Override, Override, *EntryChanged) {})

	return PackDiff{
		A:         PackIdentity{ID: a.ID, Version: a.Version, Fingerprint: a.Fingerprint},
		B:         PackIdentity{ID: b.ID, Version: b.Version, Fingerprint: b.Fingerprint},
		Identical: a.Fingerprint == b.Fingerprint,
		Sections:  sections,
		Checks:    checks,
		Overrides: overrides,
		Files:     files,
	}, nil
}

// diffEntries matches two manifest lists by id. fields lists the
// manifest-level changes; extra marks content-level ones. An id on both
// sides is reported as changed when either found something.
func diffEntries[T any](a, b []T, id func(T) string, fields func(x, y T) []FieldChange, extra func(x, y T, e *EntryChanged)) EntryDiff {
	byID := func(list []T) map[string]T {
		m := make(map[string]T, len(list))
		for _, v := range list {
			m[id(v)] = v
		}
		return m
	}
	ma, mb := byID(a), byID(b)
	out := EntryDiff{Added: []string{}, Removed: []string{}, Changed: []EntryChanged{}}
	for _, k := range slices.Sorted(maps.Keys(mb)) {
		if _, ok := ma[k]; !ok {
			out.Added = append(out.Added, k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(ma)) {
		y, ok := mb[k]
		if !ok {
			out.Removed = append(out.Removed, k)
			continue
		}
		e := EntryChanged{ID: k, Changes: fields(ma[k], y)}
		extra(ma[k], y, &e)
		if len(e.Changes) > 0 || e.ContentChanged || e.ConfigChanged {
			out.Changed = append(out.Changed, e)
		}
	}
	return out
}

// fieldChanges keeps the (field, from, to) triples whose values differ.
func fieldChanges(triples ...[3]string) []FieldChange {
	out := []FieldChange{}
	for _, t := range triples {
		if t[1] != t[2] {
			out = append(out, FieldChange{Field: t[0], From: t[1], To: t[2]})
		}
	}
	return out
}

// canonicalAppliesTo renders a scope as JSON with each list sorted, so
// a reordered list is not a change and a real change reads exactly.
func canonicalAppliesTo(a AppliesTo) string {
	sorted := AppliesTo{
		Languages: slices.Sorted(slices.Values(a.Languages)),
		Addons:    slices.Sorted(slices.Values(a.Addons)),
		Stages:    slices.Sorted(slices.Values(a.Stages)),
	}
	data, err := json.Marshal(sorted)
	if err != nil {
		// AppliesTo is three string slices; Marshal cannot fail on it.
		panic(fmt.Sprintf("standards: marshal applies_to: %v", err))
	}
	return string(data)
}

func diffFiles(a, b map[string]string) FileDiff {
	out := FileDiff{Added: []string{}, Removed: []string{}, Changed: []string{}}
	for _, p := range slices.Sorted(maps.Keys(b)) {
		if _, ok := a[p]; !ok {
			out.Added = append(out.Added, p)
		}
	}
	for _, p := range slices.Sorted(maps.Keys(a)) {
		sum, ok := b[p]
		switch {
		case !ok:
			out.Removed = append(out.Removed, p)
		case sum != a[p]:
			out.Changed = append(out.Changed, p)
		}
	}
	return out
}
