package cli

import (
	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

func newDetectCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "File-system probing: languages, unit runners, mutation tools, e2e suites, frontend files",
	}
	cmd.AddCommand(newDetectLanguagesCmd(cio))
	cmd.AddCommand(newDetectRunnersCmd(cio))
	cmd.AddCommand(newDetectMutationCmd(cio))
	cmd.AddCommand(newDetectE2ECmd(cio))
	cmd.AddCommand(newDetectFrontendCmd(cio))
	return cmd
}

// reposFromArtifact converts repo.json's repositories[] to
// []detect.Repo — the shape every internal/detect function reads.
func reposFromArtifact(artifact *repoArtifact) []detect.Repo {
	repos := make([]detect.Repo, len(artifact.Repositories))
	for i, r := range artifact.Repositories {
		repos[i] = toDetectRepo(r)
	}
	return repos
}

func loadReposForDetect(f Flags, key string) ([]detect.Repo, error) {
	if err := handoff.ValidateKey(key); err != nil {
		return nil, err
	}
	ticketDir := handoff.TicketDir(f.Home, key)
	artifact, err := loadRepoArtifact(ticketDir)
	if err != nil {
		return nil, err
	}
	return reposFromArtifact(artifact), nil
}

type detectLanguagesRepoResult struct {
	Repo      string   `json:"repo"`
	Languages []string `json:"languages"`
}

type detectLanguagesResult struct {
	Key          string                      `json:"key"`
	Repositories []detectLanguagesRepoResult `json:"repositories"`
}

func newDetectLanguagesCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "languages KEY",
		Short: "Detect each in-scope repository's languages by marker file",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("detect languages", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			f := FlagsFrom(cmd.Context())
			repos, err := loadReposForDetect(f, key)
			if err != nil {
				return Outcome{}, err
			}
			result := detectLanguagesResult{Key: key}
			for _, r := range repos {
				langs, err := detect.Languages(r)
				if err != nil {
					return Outcome{}, err
				}
				result.Repositories = append(result.Repositories, detectLanguagesRepoResult{Repo: r.Name, Languages: langs})
			}
			return Outcome{Result: result}, nil
		}),
	}
}

type detectRunnersRepoResult struct {
	Repo     string   `json:"repo"`
	Detected bool     `json:"detected"`
	Runner   string   `json:"runner,omitempty"`
	Command  string   `json:"command,omitempty"`
	Args     []string `json:"args,omitempty"`
}

type detectRunnersResult struct {
	Key          string                    `json:"key"`
	Repositories []detectRunnersRepoResult `json:"repositories"`
}

func newDetectRunnersCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "runners KEY",
		Short: "Detect each in-scope repository's unit test runner",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("detect runners", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			f := FlagsFrom(cmd.Context())
			repos, err := loadReposForDetect(f, key)
			if err != nil {
				return Outcome{}, err
			}
			result := detectRunnersResult{Key: key}
			for _, r := range repos {
				m, ok, err := detect.Runners(r)
				if err != nil {
					return Outcome{}, err
				}
				if !ok {
					result.Repositories = append(result.Repositories, detectRunnersRepoResult{Repo: r.Name, Detected: false})
					continue
				}
				result.Repositories = append(result.Repositories, detectRunnersRepoResult{
					Repo: r.Name, Detected: true, Runner: m.Runner, Command: m.Command, Args: m.Args,
				})
			}
			return Outcome{Result: result}, nil
		}),
	}
}

type detectMutationRepoResult struct {
	Repo     string `json:"repo"`
	Detected bool   `json:"detected"`
	Tool     string `json:"tool,omitempty"`
}

type detectMutationResult struct {
	Key          string                     `json:"key"`
	Repositories []detectMutationRepoResult `json:"repositories"`
}

func newDetectMutationCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "mutation KEY",
		Short: "Detect each in-scope repository's mutation testing tool",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("detect mutation", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			f := FlagsFrom(cmd.Context())
			repos, err := loadReposForDetect(f, key)
			if err != nil {
				return Outcome{}, err
			}
			result := detectMutationResult{Key: key}
			for _, r := range repos {
				m, ok, err := detect.Mutation(r)
				if err != nil {
					return Outcome{}, err
				}
				if !ok {
					result.Repositories = append(result.Repositories, detectMutationRepoResult{Repo: r.Name, Detected: false})
					continue
				}
				result.Repositories = append(result.Repositories, detectMutationRepoResult{Repo: r.Name, Detected: true, Tool: m.Tool})
			}
			return Outcome{Result: result}, nil
		}),
	}
}

type detectE2ERepoResult struct {
	Repo     string   `json:"repo"`
	Detected bool     `json:"detected"`
	Suite    string   `json:"suite,omitempty"`
	Command  string   `json:"command,omitempty"`
	Args     []string `json:"args,omitempty"`
}

type detectE2EResult struct {
	Key          string                `json:"key"`
	Repositories []detectE2ERepoResult `json:"repositories"`
}

func newDetectE2ECmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "e2e KEY",
		Short: "Detect each in-scope repository's e2e suite",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("detect e2e", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			f := FlagsFrom(cmd.Context())
			repos, err := loadReposForDetect(f, key)
			if err != nil {
				return Outcome{}, err
			}
			result := detectE2EResult{Key: key}
			for _, r := range repos {
				m, ok, err := detect.E2E(r)
				if err != nil {
					return Outcome{}, err
				}
				if !ok {
					result.Repositories = append(result.Repositories, detectE2ERepoResult{Repo: r.Name, Detected: false})
					continue
				}
				result.Repositories = append(result.Repositories, detectE2ERepoResult{
					Repo: r.Name, Detected: true, Suite: m.Suite, Command: m.Command, Args: m.Args,
				})
			}
			return Outcome{Result: result}, nil
		}),
	}
}

type detectFrontendResult struct {
	Key              string `json:"key"`
	HasFrontendFiles bool   `json:"has_frontend_files"`
}

func newDetectFrontendCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "frontend KEY",
		Short: "Report whether the ticket's in-scope files include a recognizable frontend file",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("detect frontend", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: detectFrontendResult{Key: key, HasFrontendFiles: detect.Frontend(artifact.InScopeFiles)}}, nil
		}),
	}
}
