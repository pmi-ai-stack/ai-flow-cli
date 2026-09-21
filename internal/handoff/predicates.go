package handoff

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/schema"
)

// primaryArtifacts is the one headline file per stage that
// `stage complete` checks for existence (and, when it is JSON, schema
// validity) before allowing a stage to complete — see the plan's
// design decision 5. This is deliberately narrower than StageTable's
// full Artifacts list: heading/prose rules for the Markdown files stay
// with the phase that defines them (15, 16, 23).
var primaryArtifacts = map[Stage]string{
	StageFetch:       "ticket.json",
	StageLocate:      "repo.json",
	StagePlan:        "plan.md",
	StageImplement:   "test-results.json",
	StageRenderDrift: "render-drift-results.json",
	StageChecks:      "checks-results.json",
	StageDryRun:      "dryrun-results.json",
	StageImpact:      "impact-results.json",
	StageRollback:    "rollback-results.json",
	StageInfraTest:   "infra-test-results.json",
	StageMutation:    "mutation-results.json",
	StageVisualTest:  "visual-test-results.json",
	StageE2E:         "e2e-results.json",
	StageReview:      "review.md",
	StageOutput:      "output.md",
	StageShip:        "ship-results.json",
}

// PrimaryArtifact returns stage's one headline artifact filename.
func PrimaryArtifact(stage Stage) string {
	return primaryArtifacts[stage]
}

// SchemaArtifactFor returns filename's schema.Artifacts() name and
// whether it has one. Every JSON primary artifact is schema-registered
// under its own name with the ".json" suffix trimmed (e.g.
// "render-drift-results.json" → "render-drift-results"); the five
// Markdown primary artifacts have no schema and are existence-only, so
// trimming ".json" from one of those simply fails to cut and reports
// false.
func SchemaArtifactFor(filename string) (string, bool) {
	name, isJSON := strings.CutSuffix(filename, ".json")
	if !isJSON || !slices.Contains(schema.Artifacts(), name) {
		return "", false
	}
	return name, true
}

// checksFinding is the subset of the checks-results.json finding shape
// MustFindingsOpen needs — a tolerant decode, so a real phase-14
// checks-results.json (carrying every documented field) reads
// correctly here without this package knowing about the rest of the
// shape.
type checksFinding struct {
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

type checksResults struct {
	Findings []checksFinding `json:"findings"`
}

// MustFindingsOpen reports whether ticketDir's checks-results.json (if
// it exists) carries any finding with severity "must" and status
// "open" — regardless of which stage recorded it, so a must left open
// from checks also blocks ship (design decision 6). A missing file
// means nothing has been recorded yet, which cannot block anything.
func MustFindingsOpen(ticketDir string) (bool, error) {
	data, err := ReadArtifact(filepath.Join(ticketDir, "checks-results.json"))
	if err != nil {
		return false, err
	}
	if data == nil {
		return false, nil
	}
	var results checksResults
	if err := json.Unmarshal(data, &results); err != nil {
		return false, fmt.Errorf("handoff: decode checks-results.json: %w", err)
	}
	for _, f := range results.Findings {
		if f.Severity == "must" && f.Status == "open" {
			return true, nil
		}
	}
	return false, nil
}

// OutputNotPassed reports whether the output stage's state is not
// pass — the precondition `stage approve KEY ship` enforces.
func OutputNotPassed(s *Status) bool {
	return s.Entry(StageOutput).State != StatePass
}

// ShipApproved reports whether the ship stage has a recorded approval
// — the predicate phase 5 wires into `git commit`/`git push`/
// `pr create`'s approval_required precondition.
func ShipApproved(s *Status) bool {
	return s.Entry(StageShip).Approval != nil
}
