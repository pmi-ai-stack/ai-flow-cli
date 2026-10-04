package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/host"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/host/github"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/render"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/runlog"
)

func newPRCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Open or reuse a pull request per in-scope repository",
	}
	cmd.AddCommand(newPRCreateCmd(cio))
	return cmd
}

// newGitHubTransport is buildHost's http.RoundTripper source — a
// package-level var (the same injectable pattern as newExecer/Now) so
// a test can substitute a fixture transport instead of ever making a
// real HTTP call.
var newGitHubTransport = func() http.RoundTripper { return http.DefaultTransport }

// githubToken reads AI_FLOW_GITHUB_TOKEN, else GITHUB_TOKEN, else
// GH_TOKEN (design decision 13) — the one place this fallback chain is
// spelled out; buildHost and doctor's host probe both call it, rather
// than each restating the three-variable precedence by hand.
func githubToken() string {
	if v := os.Getenv("AI_FLOW_GITHUB_TOKEN"); v != "" {
		return v
	}
	if v := os.Getenv("GITHUB_TOKEN"); v != "" {
		return v
	}
	return os.Getenv("GH_TOKEN")
}

// buildHost builds a host.Host from githubToken() — none set is
// host_auth before any HTTP call, read here exactly like buildTracker
// reads the Jira variables (phase 4): the provider package itself
// never touches os.Getenv.
func buildHost(cmd *cobra.Command, f Flags) (host.Host, error) {
	token := githubToken()
	if token == "" {
		return nil, &envelope.Error{
			Code:    "host_auth",
			Message: "AI_FLOW_GITHUB_TOKEN, GITHUB_TOKEN or GH_TOKEN must be set",
		}
	}

	verbose := runlog.From(cmd.Context()).LineWriter("github")
	return github.New(token, newGitHubTransport(), verbose).WithTimeout(hostHTTPTimeout(f)), nil
}

// hostHTTPTimeout is config.defaults.http_timeout_seconds when config
// loads, else the documented default. buildHost has never required
// config.json and does not start to here: a bounded request with the
// default deadline beats refusing a PR over an unrelated config error,
// which the command reports elsewhere if it matters.
func hostHTTPTimeout(f Flags) time.Duration {
	if cfg, err := config.Load(f.Home, f.Config); err == nil {
		return cfg.Defaults.HTTPTimeout()
	}
	return config.Defaults{}.HTTPTimeout()
}

// toRenderRepositories converts repo.json's own repository shape to
// render.Repository's trimmed one (Name/LocalPath only) — the two
// carry the same fields for this purpose, so pr create can build
// render.Data from the repo.json it already has in memory instead of
// asking render.Load to re-read and re-decode the file.
func toRenderRepositories(repos []repoArtifactRepository) []render.Repository {
	out := make([]render.Repository, len(repos))
	for i, r := range repos {
		out[i] = render.Repository{Name: r.Name, LocalPath: r.LocalPath}
	}
	return out
}

// shipResultPR mirrors one item of ship-results.schema.json's
// pull_requests[] — Number/Reused are not named in the schema's own
// properties list, but the item schema carries no
// additionalProperties: false, so they need no schema edit.
type shipResultPR struct {
	Repo   string `json:"repo"`
	URL    string `json:"url"`
	Number int    `json:"number,omitempty"`
	Draft  bool   `json:"draft,omitempty"`
	Reused bool   `json:"reused,omitempty"`
}

// shipResultsArtifact mirrors ship-results.schema.json exactly.
type shipResultsArtifact struct {
	PullRequests []shipResultPR `json:"pull_requests"`
}

func loadShipResults(ticketDir string) (*shipResultsArtifact, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "ship-results.json"))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return &shipResultsArtifact{}, nil
	}
	var s shipResultsArtifact
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("cli: decode ship-results.json: %w", err)
	}
	return &s, nil
}

// upsertPR replaces entry's own repo's record if one already exists,
// else appends it — "reuses an open PR on re-run" (catalog) extends
// to ship-results.json itself: a second `pr create` rewrites only the
// repositories it reaches, keeping every other repo's prior record.
func upsertPR(s *shipResultsArtifact, entry shipResultPR) {
	for i, e := range s.PullRequests {
		if e.Repo == entry.Repo {
			s.PullRequests[i] = entry
			return
		}
	}
	s.PullRequests = append(s.PullRequests, entry)
}

// prCreateSimulateEntry is `pr create --simulate`'s per-repository preview.
type prCreateSimulateEntry struct {
	Repo  string `json:"repo"`
	Owner string `json:"owner"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Draft bool   `json:"draft"`
}

type prCreateSimulateResult struct {
	Key          string                  `json:"key"`
	PullRequests []prCreateSimulateEntry `json:"pull_requests"`
}

func newPRCreateCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "create KEY",
		Short: "Push the branch and open (or reuse) one PR per in-scope repository",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("pr create", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			if err := requireShipApproved(f, key); err != nil {
				return Outcome{}, err
			}

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			ticket, err := loadTicket(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			title := fmt.Sprintf("%s: %s", key, ticket.Summary)

			g := buildGitOps(cfg)

			if f.Simulate {
				result := prCreateSimulateResult{Key: key}
				for _, r := range artifact.Repositories {
					owner, slug, err := g.RemoteOwnerRepo(cmd.Context(), r.LocalPath)
					if err != nil {
						return Outcome{}, err
					}
					result.PullRequests = append(result.PullRequests, prCreateSimulateEntry{
						Repo: r.Name, Owner: owner, Slug: slug, Title: title, Draft: cfg.Host.DraftPRs,
					})
				}
				return Outcome{Result: result}, nil
			}

			// Credentials are checked before any push or API call —
			// the same "fail before any side effect" ordering
			// buildTracker uses (phase 4).
			h, err := buildHost(cmd, f)
			if err != nil {
				return Outcome{}, err
			}
			body, err := loadTextArtifact(ticketDir, "pr-body.md")
			if err != nil {
				return Outcome{}, err
			}
			// "pr-body.md plus the generated summary block" (the
			// catalog's own words) — render.PRBlock shares its
			// section renderer with summary render's output.md, so
			// the two can never drift. artifact.Repositories is
			// already in memory (read above for the --simulate
			// preview too), so this builds render.Data from it
			// directly rather than having render.Load re-read and
			// re-decode repo.json a second time in this same call.
			mutation, accepted, err := render.LoadMutation(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			renderData := render.Data{Key: key, Repositories: toRenderRepositories(artifact.Repositories), Mutation: mutation, Accepted: accepted}
			body += "\n\n" + render.PRBlock(renderData)

			// Loaded once, then kept in memory across the loop and
			// written after every repository — not reloaded from disk
			// each iteration, since this command is the only writer of
			// ship-results.json within one invocation and its own last
			// write already reflects every prior repository's result.
			// Writing per repository (rather than once at the end) is
			// deliberate, not an oversight: a failure partway through
			// still leaves every already-processed repository's PR
			// recorded on disk.
			results, err := loadShipResults(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			for _, r := range artifact.Repositories {
				if err := g.Push(cmd.Context(), r.LocalPath, ticket.BranchName); err != nil {
					return Outcome{}, err
				}
				owner, slug, err := g.RemoteOwnerRepo(cmd.Context(), r.LocalPath)
				if err != nil {
					return Outcome{}, err
				}
				pr, err := h.EnsurePR(cmd.Context(), owner, slug, cfg.Host.DefaultBaseBranch, ticket.BranchName, title, body, cfg.Host.DraftPRs)
				if err != nil {
					return Outcome{}, err
				}

				upsertPR(results, shipResultPR{Repo: r.Name, URL: pr.URL, Number: pr.Number, Draft: pr.Draft, Reused: pr.Reused})
				if _, err := handoff.WriteArtifact(ticketDir, "ship-results.json", "ship-results", results); err != nil {
					return Outcome{}, err
				}
			}
			return Outcome{Result: results}, nil
		}),
	}
}
