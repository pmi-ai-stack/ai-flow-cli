package cli

import (
	"encoding/json"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/e2e"
)

// e2eResultsFailure mirrors e2e-results.schema.json's failures[] item
// exactly.
type e2eResultsFailure struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// e2eResultsArtifact mirrors e2e-results.schema.json, plus CaseNames —
// the plan's design decision 4: an extra field the schema does not
// forbid (no additionalProperties: false), read back by the next
// `e2e run` to compute NewCases.
type e2eResultsArtifact struct {
	Total     int                 `json:"total"`
	Passed    int                 `json:"passed"`
	Failed    int                 `json:"failed"`
	NewCases  []string            `json:"new_cases"`
	Failures  []e2eResultsFailure `json:"failures"`
	CaseNames []string            `json:"case_names"`
}

func newE2ECmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "Run a repository's detected e2e suite",
	}
	cmd.AddCommand(newE2ERunCmd(cio))
	return cmd
}

// findE2ERepo picks the first repository detect.E2E matches (the
// plan's design decision 11 — no --repo flag on this command).
func findE2ERepo(artifact *repoArtifact) (detect.Repo, detect.E2EMatch, bool, error) {
	for _, r := range artifact.Repositories {
		repo := toDetectRepo(r)
		m, ok, err := detect.E2E(repo)
		if err != nil {
			return detect.Repo{}, detect.E2EMatch{}, false, err
		}
		if ok {
			return repo, m, true, nil
		}
	}
	return detect.Repo{}, detect.E2EMatch{}, false, nil
}

// loadPreviousE2ECaseNames reads ticketDir's existing e2e-results.json
// (if any) for its case_names field — hasPrevious is false only when
// no file exists yet at all (e2e.Run's own design decision 4).
func loadPreviousE2ECaseNames(ticketDir string) (names []string, hasPrevious bool, err error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "e2e-results.json"))
	if err != nil {
		return nil, false, err
	}
	if data == nil {
		return nil, false, nil
	}
	var prev e2eResultsArtifact
	if err := json.Unmarshal(data, &prev); err != nil {
		return nil, false, err
	}
	return prev.CaseNames, true, nil
}

func newE2ERunCmd(cio cmdIO) *cobra.Command {
	var caseFlag string
	cmd := &cobra.Command{
		Use:   "run KEY",
		Short: "Run the detected e2e suite and write e2e-results.json",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("e2e run", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
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

			repo, match, ok, err := findE2ERepo(artifact)
			if err != nil {
				return Outcome{}, err
			}
			if !ok {
				return Outcome{}, &envelope.Error{Code: "tool_missing", Message: "no e2e suite detected in any in-scope repository"}
			}

			if f.Simulate {
				return Outcome{Result: map[string]any{"key": key, "repo": repo.Name, "suite": match.Suite, "case": caseFlag}}, nil
			}

			previousNames, hasPrevious, err := loadPreviousE2ECaseNames(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			res, err := e2e.Run(cmd.Context(), repo, match, caseFlag, previousNames, hasPrevious, newExecer(), cfg.Ops.ToolTimeout())
			if err != nil {
				return Outcome{}, err
			}

			result := e2eResultsArtifact{
				Total: res.Total, Passed: res.Passed, Failed: res.Failed,
				NewCases: res.NewCases, CaseNames: res.CaseNames,
			}
			for _, fl := range res.Failures {
				result.Failures = append(result.Failures, e2eResultsFailure{Name: fl.Name, Message: fl.Message})
			}
			if result.Failures == nil {
				result.Failures = []e2eResultsFailure{}
			}
			if result.NewCases == nil {
				result.NewCases = []string{}
			}
			if result.CaseNames == nil {
				result.CaseNames = []string{}
			}

			if _, err := handoff.WriteArtifact(ticketDir, "e2e-results.json", "e2e-results", result); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().StringVar(&caseFlag, "case", "", "narrow to e2e cases matching this filter")
	return cmd
}
