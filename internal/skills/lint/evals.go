package lint

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// evalsFile is evals/evals.json's shape checked here — a subset of
// phase 11's own evals.json schema (query/fixture/baseline/
// expected_behavior), since that schema and the harness that runs it
// are phase 11's to build. Every field phase 11 additionally requires
// will still satisfy this shape, which only asks: are there at least
// three scenarios, and do they cover all three required kinds.
type evalsFile struct {
	Scenarios []struct {
		Scenario string `json:"scenario"`
	} `json:"scenarios"`
}

var requiredEvalScenarios = []string{"happy_path", "stop_and_ask", "temptation"}

// checkEvals runs rule 16: missing-evals.
func checkEvals(s skills.Skill) []Finding {
	fail := func(msg string) []Finding {
		return []Finding{{Rule: RuleMissingEvals, Skill: s.Name, File: "evals/evals.json", Message: msg}}
	}

	path := filepath.Join(s.Dir, "evals", "evals.json")
	data, err := handoff.ReadArtifact(path)
	if err != nil || data == nil {
		return fail("evals/evals.json is missing")
	}

	var ef evalsFile
	if err := json.Unmarshal(data, &ef); err != nil {
		return fail(fmt.Sprintf("evals/evals.json is not valid JSON: %v", err))
	}
	if len(ef.Scenarios) < 3 {
		return fail(fmt.Sprintf("evals/evals.json has %d scenario(s), need at least 3", len(ef.Scenarios)))
	}

	have := map[string]bool{}
	for _, sc := range ef.Scenarios {
		have[sc.Scenario] = true
	}
	for _, want := range requiredEvalScenarios {
		if !have[want] {
			return fail(fmt.Sprintf("evals/evals.json is missing a %q scenario", want))
		}
	}
	return nil
}
