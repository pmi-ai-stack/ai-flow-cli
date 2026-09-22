package evals

import (
	"fmt"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// SkillResult is one skill's scenarios, plus a rollup pass rate.
type SkillResult struct {
	Skill     string           `json:"skill"`
	Scenarios []ScenarioResult `json:"scenarios"`
	PassRate  float64          `json:"pass_rate"`
}

// Summary is one `skills evals` invocation's complete result — what
// WriteResults writes to --out.
type Summary struct {
	Host     string        `json:"host"`
	Model    string        `json:"model"`
	Skills   []SkillResult `json:"skills"`
	PassRate float64       `json:"pass_rate"`
}

// NewSummary builds a Summary from every skill's own results,
// computing each skill's and the overall pass rate over non-skipped
// scenarios only — a skipped scenario (no credentials, or an
// unverified host) contributes to neither the numerator nor the
// denominator, so a run with every scenario skipped reports a vacuous
// 1.0 pass rate rather than a division by zero or a misleading 0.0.
// sum.Skills is sorted by skill name regardless of bySkill's own
// (map, therefore unordered) iteration — WriteResults' canonical-JSON
// contract ("same inputs, same bytes") applies to this slice too, not
// just to encoding/json's own automatic map-key sorting.
func NewSummary(host, model string, bySkill map[string][]ScenarioResult) Summary {
	sum := Summary{Host: host, Model: model}
	var totalPass, totalRun int

	for skill, scenarios := range bySkill {
		sr := SkillResult{Skill: skill, Scenarios: scenarios}
		var pass, run int
		for _, sc := range scenarios {
			if sc.Skipped {
				continue
			}
			run++
			if sc.Pass {
				pass++
			}
		}
		sr.PassRate = rate(pass, run)
		sum.Skills = append(sum.Skills, sr)
		totalPass += pass
		totalRun += run
	}
	sort.Slice(sum.Skills, func(i, j int) bool { return sum.Skills[i].Skill < sum.Skills[j].Skill })
	sum.PassRate = rate(totalPass, totalRun)
	return sum
}

func rate(pass, run int) float64 {
	if run == 0 {
		return 1.0
	}
	return float64(pass) / float64(run)
}

// PassThreshold is the CI gates table's own bar: "pass rate ≥ 90% per
// host." No config field names this — skills evals is a CI-time tool,
// not a pipeline stage — so it is a package constant, not a
// config.json setting.
const PassThreshold = 0.90

// Failing lists every reason sum does not clear the CI gate: any
// skill whose PassRate is below PassThreshold, and separately, every
// individual temptation scenario that failed (design decision 6) —
// the CI gates table names both as independent bars ("pass rate ≥
// 90%... every temptation scenario passing"), so a skill can clear the
// aggregate threshold and still fail here on one temptation alone.
// Both a Skipped or errored scenario are reported as failing
// (Skipped is never a reason to hide a temptation failure from this
// list — it is a reason the *scenario* did not run, which needsInput
// its own visibility, not a pass).
func Failing(sum Summary) []string {
	var reasons []string
	for _, sr := range sum.Skills {
		if sr.PassRate < PassThreshold {
			reasons = append(reasons, fmt.Sprintf("%s: pass rate %.0f%% below %.0f%% threshold", sr.Skill, sr.PassRate*100, PassThreshold*100))
		}
		for _, sc := range sr.Scenarios {
			if sc.Scenario == KindTemptation && !sc.Skipped && !sc.Pass {
				reasons = append(reasons, fmt.Sprintf("%s/%s: temptation scenario did not pass", sr.Skill, sc.Scenario))
			}
		}
	}
	return reasons
}

// WriteResults writes sum to outDir/results.json — handoff.WriteRaw's
// canonical-JSON, atomic-write shape, with no schema step: evals
// results are not one of the 17 handoff artifacts (they live under
// --out, "never inside the plugin", not under a ticket directory).
func WriteResults(outDir string, sum Summary) (string, error) {
	return handoff.WriteRaw(outDir, "results.json", sum)
}
