package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills/evals"
)

// defaultEvalHost and defaultEvalModel are --host/--model's own
// defaults — the CI gates table's own required cell: "one cell
// (Claude Code, Sonnet 5)."
const (
	defaultEvalHost  = "claude-code"
	defaultEvalModel = "claude-sonnet-5"
)

type skillsEvalsResult struct {
	Dir        string `json:"dir"`
	Out        string `json:"out,omitempty"`
	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skip_reason,omitempty"`
	evals.Summary
}

// newSkillsEvalsCmd builds `skills evals <dir> [--skill S] [--host H]
// [--model M] [--out D]` — see the phase 11 plan's §2 (the
// internal/skills/evals package this command drives) and design
// decision 6 (the invalid_result envelope below). ANTHROPIC_API_KEY
// absent is never a failing envelope — forks must not be blocked by a
// missing repository secret — it is ok:true with result.skipped:true,
// per the Evals section's own words: "eval jobs are skipped — not
// failed — when ANTHROPIC_API_KEY is absent."
func newSkillsEvalsCmd(cio cmdIO) *cobra.Command {
	var skillFlag, hostFlag, modelFlag, outFlag string
	var requireCredentials bool
	cmd := &cobra.Command{
		Use:   "evals <dir>",
		Short: "Run every skill's evals/evals.json scenarios against a headless host",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("skills evals", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			dir := args[0]

			if os.Getenv("ANTHROPIC_API_KEY") == "" {
				// A job that is supposed to have the key (a non-fork PR,
				// a scheduled run) says so with --require-credentials, and
				// then a missing key fails it instead of passing green
				// with nothing run. Without the flag, the spec's "skipped,
				// not failed" rule for fork PRs is unchanged.
				if requireCredentials {
					return Outcome{}, &envelope.Error{
						Code:    "credentials_missing",
						Message: "ANTHROPIC_API_KEY is not set, and --require-credentials says this run must have it",
						Details: []string{"set the ANTHROPIC_API_KEY secret for this job, or drop --require-credentials where skipping is expected (fork pull requests)"},
					}
				}
				return Outcome{Result: skillsEvalsResult{
					Dir: dir, Skipped: true, SkipReason: "ANTHROPIC_API_KEY is not set",
					Summary: evals.Summary{Skills: []evals.SkillResult{}},
				}}, nil
			}

			host, model := hostFlag, modelFlag

			discovered, err := skills.DiscoverSkills(dir)
			if err != nil {
				return Outcome{}, err
			}
			if skillFlag != "" {
				var filtered []skills.Skill
				for _, s := range discovered {
					if s.Name == skillFlag {
						filtered = append(filtered, s)
					}
				}
				if len(filtered) == 0 {
					return Outcome{}, &envelope.Error{Code: "usage", Message: fmt.Sprintf("no skill named %q under %s/skills", skillFlag, dir)}
				}
				discovered = filtered
			}

			workRoot, err := os.MkdirTemp("", "ai-flow-evals-work-")
			if err != nil {
				return Outcome{}, err
			}
			defer func() { _ = os.RemoveAll(workRoot) }()

			// The host CLIs run outside the allowlist; each run is
			// recorded in the global events.jsonl (no ticket owns it).
			x := unlistedRunnerFor(FlagsFrom(cmd.Context()), handoff.GlobalEventsPath(FlagsFrom(cmd.Context()).Home))
			bySkill := map[string][]evals.ScenarioResult{}
			for _, sk := range discovered {
				scenarios, err := evals.LoadScenarios(sk.Dir)
				if err != nil {
					return Outcome{}, &envelope.Error{Code: "invalid_result", Message: err.Error()}
				}
				results := make([]evals.ScenarioResult, 0, len(scenarios))
				for _, sc := range scenarios {
					results = append(results, evals.RunScenario(cmd.Context(), x, workRoot, dir, sk, sc, host, model))
				}
				bySkill[sk.Name] = results
			}

			sum := evals.NewSummary(host, model, bySkill)

			outDir := outFlag
			if outDir == "" {
				outDir, err = os.MkdirTemp("", "ai-flow-evals-out-")
				if err != nil {
					return Outcome{}, err
				}
			}
			resultsPath, err := evals.WriteResults(outDir, sum)
			if err != nil {
				return Outcome{}, err
			}

			if failing := evals.Failing(sum); len(failing) > 0 {
				sort.Strings(failing)
				return Outcome{}, &envelope.Error{
					Code:    "invalid_result",
					Message: fmt.Sprintf("%s: pass rate %.0f%% below the %.0f%% threshold, or a temptation scenario failed", dir, sum.PassRate*100, evals.PassThreshold*100),
					Details: failing,
				}
			}

			return Outcome{Result: skillsEvalsResult{Dir: dir, Out: resultsPath, Summary: sum}}, nil
		}),
	}
	cmd.Flags().StringVar(&skillFlag, "skill", "", "run only this skill's scenarios (default: every skill under dir/skills/)")
	cmd.Flags().StringVar(&hostFlag, "host", defaultEvalHost, "headless host to run against")
	cmd.Flags().StringVar(&modelFlag, "model", defaultEvalModel, "model to run and grade with")
	cmd.Flags().BoolVar(&requireCredentials, "require-credentials", false, "fail with credentials_missing instead of skipping when ANTHROPIC_API_KEY is absent")
	cmd.Flags().StringVar(&outFlag, "out", "", "directory to write results.json into (default: a fresh temporary directory)")
	return cmd
}
