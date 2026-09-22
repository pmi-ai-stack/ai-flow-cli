package cli

import (
	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

func newStandardsCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "standards",
		Short: "Load, validate, sync and export standards packs",
	}
	cmd.AddCommand(newStandardsValidateCmd(cio))
	cmd.AddCommand(newStandardsSyncCmd(cio))
	cmd.AddCommand(newStandardsExportCmd(cio))
	return cmd
}

type standardsValidateResult struct {
	Valid    bool     `json:"valid"`
	ID       string   `json:"id"`
	Sections int      `json:"sections"`
	Checks   int      `json:"checks"`
	Warnings []string `json:"warnings,omitempty"`
}

func newStandardsValidateCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "validate DIR",
		Short: "Validate a pack directory's standards.json against the pack schema and the loader's structural rules",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("standards validate", cio.stdout, cio.exitCode, func(_ *cobra.Command, args []string) (Outcome, error) {
			dir := args[0]
			res, err := standards.ValidateAgainstProductionAllowlist(dir)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: standardsValidateResult{
				Valid:    true,
				ID:       res.Manifest.ID,
				Sections: len(res.Manifest.Sections),
				Checks:   len(res.Manifest.Checks),
			}}, nil
		}),
	}
}

type standardsSyncedResult struct {
	ID            string `json:"id"`
	Ref           string `json:"ref"`
	RefKind       string `json:"ref_kind"`
	Path          string `json:"path"`
	Skipped       bool   `json:"skipped"`
	SkippedReason string `json:"skipped_reason,omitempty"`
}

type standardsSyncResult struct {
	Synced []standardsSyncedResult `json:"synced"`
}

func newStandardsSyncCmd(cio cmdIO) *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch every configured org and team standards source into ~/.ai-flow/standards/<id>@<ref>/",
		Args:  cobra.NoArgs,
		RunE: wrap("standards sync", cio.stdout, cio.exitCode, func(cmd *cobra.Command, _ []string) (Outcome, error) {
			f := FlagsFrom(cmd.Context())
			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			sources := standards.SourcesFromConfig(cfg)

			if f.Simulate {
				result := standardsSyncResult{}
				for _, s := range sources {
					if source != "" && s.ID != source {
						continue
					}
					result.Synced = append(result.Synced, standardsSyncedResult{
						ID: s.ID, Ref: s.Ref, Path: standards.CacheDir(f.Home, s.ID, s.Ref),
					})
				}
				return Outcome{Result: result}, nil
			}

			synced, err := standards.Sync(standards.SyncOptions{
				Home:        f.Home,
				Sources:     sources,
				Only:        source,
				GitHubToken: standards.GitHubTokenFromEnv(),
			})
			if err != nil {
				return Outcome{}, err
			}

			result := standardsSyncResult{}
			for _, s := range synced {
				result.Synced = append(result.Synced, standardsSyncedResult{
					ID: s.ID, Ref: s.Ref, RefKind: string(s.RefKind), Path: s.Path,
					Skipped: s.Skipped, SkippedReason: s.SkippedReason,
				})
				_ = handoff.AppendEvent(handoff.GlobalEventsPath(f.Home), handoff.StandardsSyncEvent(s.ID, s.Ref, string(s.RefKind), !s.Skipped))
			}
			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().StringVar(&source, "source", "", "sync only the source with this id")
	return cmd
}

type standardsExportResult struct {
	Path string `json:"path"`
}

func newStandardsExportCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "export DIR",
		Short: "Write the embedded reference pack out to DIR, as a starting point to fork",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("standards export", cio.stdout, cio.exitCode, func(_ *cobra.Command, args []string) (Outcome, error) {
			dir := args[0]
			if err := standards.Export(dir); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: standardsExportResult{Path: dir}}, nil
		}),
	}
}
