package standards

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/schema"
)

// TestWriteArtifactSatisfiesItsSchema proves the standards.json this
// phase writes matches the schema this phase tightened — the two are
// written together and must not drift apart.
func TestWriteArtifactSatisfiesItsSchema(t *testing.T) {
	e, _ := resolveFixture(t, true, 6000, beforeWaiverExpiry)
	dir := t.TempDir()

	path, err := WriteArtifact(dir, e)
	if err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := schema.Validate("standards", data); err != nil {
		t.Fatalf("the written standards.json does not satisfy its own schema: %v", err)
	}
	if filepath.Base(path) != ArtifactFile {
		t.Errorf("WriteArtifact wrote %s, want a file named %s", path, ArtifactFile)
	}
}

// TestArtifactCarriesEveryAnnotation proves standards.json records
// what the spec's handoff table says it records: "effective sections
// and checks with overridden_by, waived, dropped_from[], resolution
// hash".
func TestArtifactCarriesEveryAnnotation(t *testing.T) {
	e, _ := resolveFixture(t, true, 20, beforeWaiverExpiry)
	a := ToArtifact(e)

	if a.ResolutionHash != e.Hash {
		t.Errorf("ResolutionHash = %q, want %q", a.ResolutionHash, e.Hash)
	}

	var sawOverriddenBy, sawEffective, sawDroppedFrom, sawConflict bool
	for _, s := range a.Sections {
		switch {
		case s.OverriddenBy != nil && *s.OverriddenBy != "":
			sawOverriddenBy = true
		case s.OverriddenBy == nil:
			sawEffective = true
		}
		if len(s.DroppedFrom) > 0 {
			sawDroppedFrom = true
		}
		if s.Conflict != "" {
			sawConflict = true
		}
	}
	for name, ok := range map[string]bool{
		"an overridden_by annotation": sawOverriddenBy,
		"an effective (null) entry":   sawEffective,
		"a dropped_from annotation":   sawDroppedFrom,
		"a conflict annotation":       sawConflict,
	} {
		if !ok {
			t.Errorf("standards.json sections[] carries no %s", name)
		}
	}

	var sawWaived bool
	for _, c := range a.Checks {
		if c.Waived {
			sawWaived = true
			if c.Waiver == nil {
				t.Errorf("check %s is waived but carries no waiver record", c.ID)
			}
		}
	}
	if !sawWaived {
		t.Error("standards.json checks[] carries no waived check")
	}
	if len(a.Conflicts) == 0 {
		t.Error("standards.json carries no conflicts[], but the fixture has an undeclared override")
	}
	if len(a.Waivers) == 0 {
		t.Error("standards.json carries no waivers[], but the fixture repository has two")
	}
}

// TestArtifactByteStable is the determinism invariant applied to this
// phase's own artifact: two independent resolutions of identical inputs
// serialize to identical bytes.
func TestArtifactByteStable(t *testing.T) {
	first, _ := resolveFixture(t, true, 6000, beforeWaiverExpiry)
	second, _ := resolveFixture(t, true, 6000, beforeWaiverExpiry)

	a, err := json.MarshalIndent(ToArtifact(first), "", "  ")
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	b, err := json.MarshalIndent(ToArtifact(second), "", "  ")
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("two independent resolutions serialized differently:\n--- first ---\n%s\n--- second ---\n%s", a, b)
	}
}

// TestReadArtifactHashRoundTrips proves the idempotency comparison
// both `standards resolve` and `stage start` rely on.
func TestReadArtifactHashRoundTrips(t *testing.T) {
	dir := t.TempDir()

	got, err := ReadArtifactHash(dir)
	if err != nil {
		t.Fatalf("ReadArtifactHash on an empty directory: %v", err)
	}
	if got != "" {
		t.Errorf("ReadArtifactHash on an empty directory = %q, want \"\"", got)
	}

	e, _ := resolveFixture(t, true, 6000, beforeWaiverExpiry)
	if _, err := WriteArtifact(dir, e); err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	got, err = ReadArtifactHash(dir)
	if err != nil {
		t.Fatalf("ReadArtifactHash: %v", err)
	}
	if got != e.Hash {
		t.Errorf("ReadArtifactHash = %q, want %q", got, e.Hash)
	}

	// An unparseable standards.json reads as absent rather than fatal:
	// the next resolve overwrites it anyway, and refusing to resolve
	// because the previous resolution is unreadable would strand the
	// ticket.
	writeFile(t, filepath.Join(dir, ArtifactFile), "{not json")
	got, err = ReadArtifactHash(dir)
	if err != nil || got != "" {
		t.Errorf("ReadArtifactHash on a corrupt file = (%q, %v), want (\"\", nil)", got, err)
	}
}
