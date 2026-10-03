package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

func newStandardsCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "standards",
		Short: "Sync, validate and resolve standards packs, and waive a check",
	}
	cmd.AddCommand(newStandardsValidateCmd(cio))
	cmd.AddCommand(newStandardsSyncCmd(cio))
	cmd.AddCommand(newStandardsExportCmd(cio))
	cmd.AddCommand(newStandardsResolveCmd(cio))
	cmd.AddCommand(newStandardsShowCmd(cio))
	cmd.AddCommand(newStandardsWaiveCmd(cio))
	cmd.AddCommand(newStandardsCheckCmd(cio))
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

			synced, err := standards.Sync(cmd.Context(), standards.SyncOptions{
				Home:        f.Home,
				Sources:     sources,
				Only:        source,
				GitHubToken: standards.GitHubTokenFromEnv(),
				Timeout:     standards.DefaultSyncTimeout,
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

// standardsResolveResult is `standards resolve`'s result shape. It
// reports the resolution, not the guidance: guidance.md is per stage
// and is written by `stage start`, which knows which stage is
// starting.
type standardsResolveResult struct {
	Key            string                  `json:"key"`
	ResolutionHash string                  `json:"resolution_hash"`
	Unchanged      bool                    `json:"unchanged"`
	Languages      []string                `json:"languages"`
	Addons         []string                `json:"addons"`
	Packs          []standardsPackResult   `json:"packs"`
	Sections       int                     `json:"sections"`
	Checks         int                     `json:"checks"`
	Waived         []string                `json:"waived"`
	Conflicts      []standardsConflictItem `json:"conflicts"`
}

type standardsPackResult struct {
	ID     string `json:"id"`
	Layer  string `json:"layer"`
	Ref    string `json:"ref"`
	Commit string `json:"commit,omitempty"`
}

type standardsConflictItem struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Rule   string `json:"rule"`
	Winner string `json:"winner"`
	Loser  string `json:"loser"`
}

func newStandardsResolveCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "resolve KEY",
		Short: "Resolve the org, team and repo standards layers for a ticket and write standards.json",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("standards resolve", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}

			if f.Simulate {
				e, packs, err := resolveForTicket(f, key, cfg)
				defer standards.ClosePacks(packs)
				if err != nil {
					return Outcome{}, err
				}
				return Outcome{Result: toResolveResult(key, e, true), Warnings: e.Warnings()}, nil
			}

			e, packs, written, err := resolveAndWrite(f, key, cfg)
			defer standards.ClosePacks(packs)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: toResolveResult(key, e, !written), Warnings: e.Warnings()}, nil
		}),
	}
}

func toResolveResult(key string, e *standards.Effective, unchanged bool) standardsResolveResult {
	res := standardsResolveResult{
		Key: key, ResolutionHash: e.Hash, Unchanged: unchanged,
		Languages: e.Languages, Addons: e.Addons,
		Packs: make([]standardsPackResult, 0, len(e.Packs)),
		// Both stay non-nil so the envelope always carries an array —
		// 10-data-types.md's "ordered data is an array", including when
		// it is empty.
		Waived:    []string{},
		Conflicts: []standardsConflictItem{},
	}
	for _, p := range e.Packs {
		res.Packs = append(res.Packs, standardsPackResult{ID: p.ID, Layer: p.Layer, Ref: p.Ref, Commit: p.Commit})
	}
	for _, s := range e.Sections {
		if s.Effective() {
			res.Sections++
		}
	}
	for _, c := range e.Checks {
		if !c.Effective() {
			continue
		}
		res.Checks++
		if c.Waived {
			res.Waived = append(res.Waived, c.ID)
		}
	}
	for _, c := range e.Conflicts {
		res.Conflicts = append(res.Conflicts, standardsConflictItem{
			ID: c.ID, Kind: c.Kind, Rule: c.Rule, Winner: c.Winner, Loser: c.Loser,
		})
	}
	return res
}

// standardsShowResult is `standards show`'s result: one section's full
// content, which is how a skill reads a section the budget dropped
// from its stage's guidance.md.
type standardsShowResult struct {
	ID          string   `json:"id"`
	Layer       string   `json:"layer"`
	Severity    string   `json:"severity"`
	Path        string   `json:"path"`
	DroppedFrom []string `json:"dropped_from"`
	Content     string   `json:"content"`
}

func newStandardsShowCmd(cio cmdIO) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "show KEY",
		Short: "Print one resolved guidance section in full, dropped by the budget or not",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("standards show", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			if id == "" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--id is required"}
			}
			f := FlagsFrom(cmd.Context())
			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}

			e, packs, err := resolveForTicket(f, key, cfg)
			defer standards.ClosePacks(packs)
			if err != nil {
				return Outcome{}, err
			}

			s, ok := e.Section(id)
			if !ok {
				return Outcome{}, &envelope.Error{
					Code:    "usage",
					Message: fmt.Sprintf("%s resolves no section with id %q", key, id),
					Details: effectiveSectionIDs(e),
				}
			}
			body, err := s.Pack.ReadSection(s.Section)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: standardsShowResult{
				ID: s.ID, Layer: s.Layer, Severity: s.Severity, Path: s.Path,
				DroppedFrom: s.DroppedFrom, Content: string(body),
			}}, nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "the section id to print (required)")
	return cmd
}

// effectiveSectionIDs lists every id `standards show --id` would
// accept, for the unknown-id error's details — the same "one of:
// <list>" shape `schema <artifact>` and `stage <stage>` already use.
func effectiveSectionIDs(e *standards.Effective) []string {
	var out []string
	for _, s := range e.Sections {
		if s.Effective() {
			out = append(out, s.ID)
		}
	}
	sort.Strings(out)
	return out
}

// standardsWaiveResult is `standards waive`'s result.
type standardsWaiveResult struct {
	Key     string `json:"key"`
	CheckID string `json:"check_id"`
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Expires string `json:"expires"`
	Written bool   `json:"written"`
}

func newStandardsWaiveCmd(cio cmdIO) *cobra.Command {
	var check, repoName, reason, expires string
	var confirmed bool
	cmd := &cobra.Command{
		Use:   "waive KEY",
		Short: "Waive one check for one repository, until a required expiry date",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("standards waive", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			if err := requireWaiveFlags(check, repoName, reason, expires); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}

			e, packs, err := resolveForTicket(f, key, cfg)
			defer standards.ClosePacks(packs)
			if err != nil {
				return Outcome{}, err
			}

			target, err := waiveTargetRepo(f, key, repoName)
			if err != nil {
				return Outcome{}, err
			}
			expiry, err := validateWaiveCheck(e, check, expires)
			if err != nil {
				return Outcome{}, err
			}

			result := standardsWaiveResult{
				Key: key, CheckID: check, Repo: target.Name,
				Path: standards.WaiversPath(target.LocalPath), Expires: expiry,
			}

			// The confirmation is the whole point of the command: a
			// waiver is a human decision to let a `must` finding pass,
			// so the CLI asks once, through the only mechanism it has
			// for asking anything — exit 4 with a resume_with the skill
			// relays and replays.
			if !confirmed {
				resumeWith := fmt.Sprintf(
					"standards waive %s --check %s --repo %s --reason %q --expires %s --confirmed",
					key, check, target.Name, reason, expires)
				needsInput := &envelope.NeedsInput{
					Question:   fmt.Sprintf("Waive check %s for %s until %s? Reason: %s", check, target.Name, expiry, reason),
					Options:    []string{"confirm", "cancel"},
					ResumeWith: resumeWith,
				}
				return Outcome{NeedsInput: needsInput}, &envelope.Error{
					Code:    "waiver_confirmation",
					Message: fmt.Sprintf("waiving %s for %s needs --confirmed", check, target.Name),
				}
			}

			if f.Simulate {
				return Outcome{Result: result}, nil
			}
			if err := standards.AddWaiver(target.LocalPath, standards.Waiver{
				CheckID: check, Reason: reason, Author: currentUser(),
				Created: handoff.Now().UTC().Format(time.RFC3339), Expires: expiry,
			}); err != nil {
				return Outcome{}, err
			}
			result.Written = true
			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().StringVar(&check, "check", "", "the check id to waive (required)")
	cmd.Flags().StringVar(&repoName, "repo", "", "the in-scope repository whose waivers file to write (required)")
	cmd.Flags().StringVar(&reason, "reason", "", "why this check is being waived (required)")
	cmd.Flags().StringVar(&expires, "expires", "", "when the waiver expires, RFC 3339 or YYYY-MM-DD (required)")
	cmd.Flags().BoolVar(&confirmed, "confirmed", false, "the developer confirmed; without this the command exits 4 waiver_confirmation")
	return cmd
}

// requireWaiveFlags enforces the four flags that carry no sensible
// default. `--expires` is among them deliberately: the spec makes the
// expiry required, because a waiver without one is a silent permanent
// suppression.
func requireWaiveFlags(check, repoName, reason, expires string) error {
	var missing []string
	for _, f := range []struct {
		name  string
		value string
	}{
		{"--check", check}, {"--repo", repoName}, {"--reason", reason}, {"--expires", expires},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return &envelope.Error{
			Code:    "usage",
			Message: strings.Join(missing, ", ") + " are required",
		}
	}
	return nil
}

// waiveTargetRepo resolves --repo against the ticket's own in-scope
// repositories — a waiver belongs in exactly one repository's
// checkout, and naming one that is not in scope is a developer
// mistake, not a missing tool.
func waiveTargetRepo(f Flags, key, repoName string) (repoArtifactRepository, error) {
	artifact, err := loadRepoArtifact(handoff.TicketDir(f.Home, key))
	if err != nil {
		return repoArtifactRepository{}, err
	}
	matched, err := filterReposByName(artifact.Repositories, repoName)
	if err != nil {
		return repoArtifactRepository{}, err
	}
	return matched[0], nil
}

// validateWaiveCheck refuses the three cases the spec and the exit-code
// contract between them make unwaivable, all as `usage` (exit 2): they
// are invocation mistakes the CLI detects before touching anything,
// which is exit 2's own definition. Exit 4 `waiver_confirmation` stays
// reserved for its one documented use, the missing --confirmed.
func validateWaiveCheck(e *standards.Effective, check, expires string) (string, error) {
	c, ok := e.Check(check)
	if !ok {
		return "", &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("no resolved check with id %q", check),
			Details: []string{"sections cannot be waived, only overridden"},
		}
	}
	if c.Kind == standards.UnwaivableKind {
		return "", &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("check %s is a %s check and cannot be waived", check, standards.UnwaivableKind),
		}
	}

	expiry, err := standards.ParseExpiry(expires)
	if err != nil {
		return "", &envelope.Error{Code: "usage", Message: "--expires: " + err.Error()}
	}
	if !expiry.After(handoff.Now().UTC()) {
		return "", &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("--expires %s is not in the future; an already-expired waiver re-fires immediately", expires),
		}
	}
	return expiry.Format(time.RFC3339), nil
}
