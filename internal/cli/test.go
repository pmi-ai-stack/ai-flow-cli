package cli

import (
	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/unit"
)

// testResultsFailure mirrors test-results.schema.json's failures[]
// item exactly.
type testResultsFailure struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// testResultsRepo mirrors test-results.schema.json's repositories[]
// item exactly. TreeHash is set only by mutation run's own pre-check
// (internal/cli/mutation.go's ensureMutationTargetsTestsGreen) — never
// by `test run` itself (the plan's phase-7 design decision 7), so it
// stays empty (and omitted, via omitempty) on every artifact this file
// writes.
type testResultsRepo struct {
	Repo     string               `json:"repo"`
	Runner   string               `json:"runner"`
	Total    int                  `json:"total"`
	Passed   int                  `json:"passed"`
	Failed   int                  `json:"failed"`
	TreeHash string               `json:"tree_hash,omitempty"`
	Failures []testResultsFailure `json:"failures"`
}

// testResultsArtifact mirrors test-results.schema.json exactly.
type testResultsArtifact struct {
	Repositories []testResultsRepo `json:"repositories"`
}

func toTestResultsRepo(r unit.Result) testResultsRepo {
	out := testResultsRepo{Repo: r.Repo, Runner: r.Runner, Total: r.Total, Passed: r.Passed, Failed: r.Failed}
	for _, f := range r.Failures {
		out.Failures = append(out.Failures, testResultsFailure{Name: f.Name, Message: f.Message})
	}
	if out.Failures == nil {
		out.Failures = []testResultsFailure{}
	}
	return out
}

func newTestCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run a repository's detected unit test suite",
	}
	cmd.AddCommand(newTestRunCmd(cio))
	return cmd
}

func newTestRunCmd(cio cmdIO) *cobra.Command {
	var repoFlag string
	cmd := &cobra.Command{
		Use:   "run KEY",
		Short: "Run the detected unit test runner per in-scope repository, and write test-results.json",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("test run", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			targets, err := filterReposByName(artifact.Repositories, repoFlag)
			if err != nil {
				return Outcome{}, err
			}

			x := newExecer()
			timeout := cfg.Defaults.TestTimeout()
			var warnings []string
			result := testResultsArtifact{}
			for _, r := range targets {
				repo := toDetectRepo(r)
				m, ok, err := detect.Runners(repo)
				if err != nil {
					return Outcome{}, err
				}
				if !ok {
					if repoFlag != "" {
						return Outcome{}, &envelope.Error{Code: "tool_missing", Message: "no unit test runner detected for " + r.Name}
					}
					warnings = append(warnings, "no unit test runner detected for "+r.Name+"; skipped")
					continue
				}

				if f.Simulate {
					result.Repositories = append(result.Repositories, testResultsRepo{Repo: r.Name, Runner: m.Runner, Failures: []testResultsFailure{}})
					continue
				}

				res, err := unit.Run(cmd.Context(), repo, m, x, timeout)
				if err != nil {
					return Outcome{}, err
				}
				result.Repositories = append(result.Repositories, toTestResultsRepo(res))
			}
			if result.Repositories == nil {
				result.Repositories = []testResultsRepo{}
			}

			if !f.Simulate {
				if _, err := handoff.WriteArtifact(ticketDir, "test-results.json", "test-results", result); err != nil {
					return Outcome{}, err
				}
			}
			return Outcome{Result: result, Warnings: warnings}, nil
		}),
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "narrow to one repository by name")
	return cmd
}
