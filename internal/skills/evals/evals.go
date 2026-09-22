// Package evals is the harness `skills evals` drives: it loads
// evals/evals.json's full schema (a strict superset of
// internal/skills/lint's own missing-evals shape check), stages a
// scenario's recorded fixture under a temporary --home (see the phase
// 11 plan's design decision 2), runs it against a headless host,
// grades the transcript, checks events.jsonl for the expected command
// order, and writes one Summary. Every real subprocess goes through
// internal/ops/exec's UnlistedRunner — the same "outside the
// allowlist" seam internal/verify/visual's dev server already
// established — so every test in this package runs against a scripted
// fake, never a real `claude` binary (see the plan's §1.3).
package evals

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// ScenarioKind is one of the three scenario kinds every evals.json must
// cover at least once.
type ScenarioKind string

const (
	KindHappyPath  ScenarioKind = "happy_path"
	KindStopAndAsk ScenarioKind = "stop_and_ask"
	KindTemptation ScenarioKind = "temptation"
)

// requiredKinds is every kind an evals.json must name at least one
// scenario of — the same three internal/skills/lint's own
// requiredEvalScenarios names, kept as an independent literal here
// rather than imported, since internal/skills/lint is a sibling
// package this one has no reason to depend on (see the plan's §1.2:
// lint's shape check is untouched by this phase).
var requiredKinds = []ScenarioKind{KindHappyPath, KindStopAndAsk, KindTemptation}

// Scenario is one evals.json scenario, decoded and validated against
// the Evals section's full field list.
type Scenario struct {
	Kind             ScenarioKind
	Query            string
	Fixture          string
	Baseline         string
	ExpectedBehavior []string
}

// scenarioFile is evals/evals.json's on-disk shape.
type scenarioFile struct {
	Scenarios []struct {
		Scenario         string   `json:"scenario"`
		Query            string   `json:"query"`
		Fixture          string   `json:"fixture"`
		Baseline         string   `json:"baseline"`
		ExpectedBehavior []string `json:"expected_behavior"`
	} `json:"scenarios"`
}

// LoadScenarios reads and validates skillDir/evals/evals.json: at
// least three scenarios, all four fields non-empty on every one, and
// every one of requiredKinds present at least once. A file that fails
// internal/skills/lint's own missing-evals rule fails here too (this
// is a strict superset of that shape check, never a looser one), but a
// file lint accepts may still fail here — LoadScenarios also demands
// non-empty query/fixture/baseline/expected_behavior, which lint's own
// shape check does not.
func LoadScenarios(skillDir string) ([]Scenario, error) {
	path := filepath.Join(skillDir, "evals", "evals.json")
	data, err := handoff.ReadArtifact(path)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("evals: %s is missing", path)
	}

	var sf scenarioFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("evals: %s is not valid JSON: %w", path, err)
	}
	if len(sf.Scenarios) < 3 {
		return nil, fmt.Errorf("evals: %s has %d scenario(s), need at least 3", path, len(sf.Scenarios))
	}

	have := map[ScenarioKind]bool{}
	scenarios := make([]Scenario, 0, len(sf.Scenarios))
	for i, raw := range sf.Scenarios {
		kind := ScenarioKind(raw.Scenario)
		if !validKind(kind) {
			return nil, fmt.Errorf("evals: %s scenario %d: %q is not a recognized scenario kind", path, i, raw.Scenario)
		}
		if raw.Query == "" || raw.Fixture == "" || raw.Baseline == "" || len(raw.ExpectedBehavior) == 0 {
			return nil, fmt.Errorf("evals: %s scenario %d (%s): query, fixture, baseline and expected_behavior are all required and non-empty", path, i, kind)
		}
		have[kind] = true
		scenarios = append(scenarios, Scenario{
			Kind:             kind,
			Query:            raw.Query,
			Fixture:          raw.Fixture,
			Baseline:         raw.Baseline,
			ExpectedBehavior: raw.ExpectedBehavior,
		})
	}

	for _, want := range requiredKinds {
		if !have[want] {
			return nil, fmt.Errorf("evals: %s is missing a %q scenario", path, want)
		}
	}
	return scenarios, nil
}

func validKind(k ScenarioKind) bool {
	switch k {
	case KindHappyPath, KindStopAndAsk, KindTemptation:
		return true
	default:
		return false
	}
}
