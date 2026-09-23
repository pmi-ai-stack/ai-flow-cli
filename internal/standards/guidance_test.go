package standards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGuidanceByteStable is the spec's own "guidance.md byte-stable"
// test, and the byte-for-byte half of the phase's Done when: the same
// resolution inputs render the same bytes, every time.
func TestGuidanceByteStable(t *testing.T) {
	first, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)
	a, err := RenderGuidance(first, "PROJ-1", "implement")
	if err != nil {
		t.Fatalf("RenderGuidance: %v", err)
	}

	// A second, completely independent resolution — its own temp home,
	// its own copy of every fixture — so this proves determinism across
	// runs, not merely that one *Effective renders consistently.
	second, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)
	b, err := RenderGuidance(second, "PROJ-1", "implement")
	if err != nil {
		t.Fatalf("RenderGuidance (second run): %v", err)
	}

	if string(a) != string(b) {
		t.Errorf("two independent resolutions rendered different bytes:\n--- first ---\n%s\n--- second ---\n%s", a, b)
	}
	if first.Hash != second.Hash {
		t.Errorf("resolution hash differs across runs: %s vs %s", first.Hash, second.Hash)
	}
}

// TestGuidanceLayout pins the Guidance delivery section's documented
// shape: the header names the packs and commits, each section appears
// under `## [severity] id`, and the `Dropped by budget:` line is
// always present.
func TestGuidanceLayout(t *testing.T) {
	e, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)
	body, err := RenderGuidance(e, "PROJ-1", "implement")
	if err != nil {
		t.Fatalf("RenderGuidance: %v", err)
	}
	got := string(body)

	for _, want := range []string{
		"# Guidance — PROJ-1 · implement\n",
		"Packs: acme-org@v3.2.0 (",
		"Languages: go\n",
		"Addons: (none)\n",
		"Budget: 0 tokens\n",
		"## [must] repo.instructions\n",
		"## [must] org.errors\n",
		"## [should] org.logging\n",
		"Dropped by budget: (none)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("guidance.md is missing %q:\n%s", want, got)
		}
	}

	// The repo pack's own org.errors body, not the org pack's — the
	// declared override won.
	if !strings.Contains(got, "platform's trace-carrying type") {
		t.Error("guidance.md carries the org org.errors body; the declared repo override should have won")
	}
	// Severity order: every must before every should.
	if strings.Index(got, "## [should]") < strings.LastIndex(got, "## [must]") {
		t.Error("guidance.md interleaves should sections among the musts")
	}
}

// TestGuidanceDroppedAndOverflowLines proves the two budget-driven
// lines: the dropped-id list, and the must-overflow record that the
// spec requires instead of truncating anything.
func TestGuidanceDroppedAndOverflowLines(t *testing.T) {
	// A 20-token budget is far below the fixture's must set alone, so
	// every must is still rendered and everything else is dropped.
	e, _ := resolveFixture(t, true, 20, beforeWaiverExpiry)
	body, err := RenderGuidance(e, "PROJ-1", "implement")
	if err != nil {
		t.Fatalf("RenderGuidance: %v", err)
	}
	got := string(body)

	if !strings.Contains(got, "Budget exceeded by must sections: ") {
		t.Errorf("guidance.md does not record the must overflow:\n%s", got)
	}
	for _, mustSection := range []string{"## [must] repo.instructions", "## [must] org.errors", "## [must] org.security"} {
		if !strings.Contains(got, mustSection) {
			t.Errorf("guidance.md dropped %q; nothing may truncate a must", mustSection)
		}
	}
	if strings.Contains(got, "## [should]") || strings.Contains(got, "## [may]") {
		t.Error("guidance.md rendered a should or a may below an overflowing must set")
	}
	if !strings.Contains(got, "Dropped by budget: org.logging") {
		t.Errorf("guidance.md's dropped line does not name the dropped shoulds:\n%s", got)
	}
}

// TestGuidancePerStage proves the per-stage-budget decision: the same
// resolution renders different sections for different stages, because
// applies_to.stages narrows them.
func TestGuidancePerStage(t *testing.T) {
	e, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)

	implement, err := RenderGuidance(e, "PROJ-1", "implement")
	if err != nil {
		t.Fatalf("RenderGuidance(implement): %v", err)
	}
	plan, err := RenderGuidance(e, "PROJ-1", "plan")
	if err != nil {
		t.Fatalf("RenderGuidance(plan): %v", err)
	}

	if !strings.Contains(string(implement), "## [must] repo.instructions") {
		t.Error("the implement stage's guidance is missing repo.instructions")
	}
	if strings.Contains(string(plan), "## [must] repo.instructions") {
		t.Error("the plan stage's guidance carries repo.instructions, which applies only to implement and review")
	}
	if !strings.Contains(string(plan), "# Guidance — PROJ-1 · plan") {
		t.Error("the plan stage's guidance does not name its own stage")
	}
}

// TestWriteGuidanceRoundTrips proves the writer lands the same bytes
// RenderGuidance produced, atomically, at the documented filename.
func TestWriteGuidanceRoundTrips(t *testing.T) {
	e, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)
	dir := t.TempDir()

	path, err := WriteGuidance(dir, "PROJ-1", "implement", e)
	if err != nil {
		t.Fatalf("WriteGuidance: %v", err)
	}
	if filepath.Base(path) != GuidanceFile {
		t.Errorf("WriteGuidance wrote %s, want a file named %s", path, GuidanceFile)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	rendered, err := RenderGuidance(e, "PROJ-1", "implement")
	if err != nil {
		t.Fatalf("RenderGuidance: %v", err)
	}
	if string(onDisk) != string(rendered) {
		t.Error("the written guidance.md differs from RenderGuidance's own output")
	}
}

// TestNormalizeBodyIsWhatTheBudgetMeasures pins the invariant that
// keeps the budget honest: the bytes counted are the bytes rendered.
func TestNormalizeBodyIsWhatTheBudgetMeasures(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"body", "body\n"},
		{"body\n", "body\n"},
		{"body\n\n\n", "body\n"},
		{"\n\n  body  \n\n", "body\n"},
		{"   ", "\n"},
	} {
		if got := string(normalizeBody([]byte(tc.in))); got != tc.want {
			t.Errorf("normalizeBody(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	e, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)
	for _, s := range e.Sections {
		if !s.Effective() {
			continue
		}
		body, err := s.Pack.ReadSection(s.Section)
		if err != nil {
			t.Fatalf("ReadSection %s: %v", s.ID, err)
		}
		if want := len(normalizeBody(body)); s.Bytes != want {
			t.Errorf("section %s Bytes = %d, want %d (the rendered length)", s.ID, s.Bytes, want)
		}
	}
}
