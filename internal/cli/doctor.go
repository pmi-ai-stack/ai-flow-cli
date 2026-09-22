package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/host"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/host/github"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/tracker/jira"
)

// doctorEnvVar reports whether one environment variable is set, by
// name only — never its value (07-determinism.md: secrets are
// redacted everywhere; the cheapest way to guarantee that here is to
// never read the value into the result at all).
type doctorEnvVar struct {
	Name string `json:"name"`
	Set  bool   `json:"set"`
}

type doctorConfig struct {
	Path     string   `json:"path"`
	Exists   bool     `json:"exists"`
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type doctorTracker struct {
	CredsSet        bool     `json:"creds_set"`
	Reachable       *bool    `json:"reachable"`
	Error           string   `json:"error,omitempty"`
	MissingStatuses []string `json:"missing_statuses,omitempty"`
}

type doctorHost struct {
	CredsSet  bool   `json:"creds_set"`
	Reachable *bool  `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

type doctorTool struct {
	Name  string `json:"name"`
	Found bool   `json:"found"`
	Path  string `json:"path,omitempty"`
}

type doctorActive struct {
	Key                 string `json:"key"`
	Stage               string `json:"stage"`
	LastEventAt         string `json:"last_event_at"`
	Stale               bool   `json:"stale"`
	DifferentCLIVersion bool   `json:"different_cli_version"`
}

// doctorHostReport is one agent host's Detect result, phase 9's own
// "plugin roots found" line.
type doctorHostReport struct {
	Host       string `json:"host"`
	Detected   bool   `json:"detected"`
	PluginRoot string `json:"plugin_root,omitempty"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
}

// doctorSelftestReport is one host's `hook selftest` result — the same
// RunHookSelftest hook.go's own CLI command uses, called directly
// rather than shelled out to.
type doctorSelftestReport struct {
	Host   string `json:"host"`
	Passed bool   `json:"passed"`
}

// doctorPackReport is one standards source's health — phase 12's own
// "pack block: id, ref, sync state, validation result" line in the
// growing-surfaces registry.
type doctorPackReport struct {
	ID     string `json:"id"`
	Layer  string `json:"layer"`
	Repo   string `json:"repo,omitempty"`
	Ref    string `json:"ref"`
	Synced bool   `json:"synced"`
	Stale  bool   `json:"stale"`
	Valid  *bool  `json:"valid"`
	Error  string `json:"error,omitempty"`
}

type doctorResult struct {
	Config          doctorConfig           `json:"config"`
	EnvVars         []doctorEnvVar         `json:"env_vars"`
	Tracker         doctorTracker          `json:"tracker"`
	Host            doctorHost             `json:"host"`
	Git             doctorTool             `json:"git"`
	TestRunners     []doctorTool           `json:"test_runners"`
	MutationTools   []doctorTool           `json:"mutation_tools"`
	Active          *doctorActive          `json:"active"`
	LastSessionHost *string                `json:"last_session_host"`
	Hosts           []doctorHostReport     `json:"hosts"`
	HookSelftest    []doctorSelftestReport `json:"hook_selftest"`
	Packs           []doctorPackReport     `json:"packs"`
}

// testRunnerBinaries and mutationToolBinaries are the fixed tables
// doctor probes with exec.Which — the same binaries
// internal/detect.Runners/Mutation name for the four language
// ecosystems this codebase supports today. doctor is not ticket-scoped
// (no repo.json to detect against), so a fixed table is the only
// detection-free reading of "test and mutation runners" available.
var testRunnerBinaries = []string{"go", "npx", "pytest"}
var mutationToolBinaries = []string{"gremlins", "npx", "mutmut", "mvn"}

func newDoctorCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report on the health of ai-flow's configuration, credentials and tools",
		Args:  cobra.NoArgs,
		RunE: wrap("doctor", cio.stdout, cio.exitCode, func(cmd *cobra.Command, _ []string) (Outcome, error) {
			f := FlagsFrom(cmd.Context())
			result := doctorResult{}
			var warnings []string

			cfg := doctorLoadConfig(f, &result.Config)
			warnings = append(warnings, doctorConfigWarnings(result.Config)...)

			result.EnvVars = doctorEnvVars()

			result.Tracker = doctorProbeTracker(cmd.Context(), cfg)
			if result.Tracker.CredsSet && result.Tracker.Reachable != nil && !*result.Tracker.Reachable {
				warnings = append(warnings, "tracker: "+result.Tracker.Error)
			}
			for _, s := range result.Tracker.MissingStatuses {
				warnings = append(warnings, "tracker: configured status \""+s+"\" was not found in the tracker's workflow")
			}

			result.Host = doctorProbeHost(cmd.Context())
			if result.Host.CredsSet && result.Host.Reachable != nil && !*result.Host.Reachable {
				warnings = append(warnings, "host: "+result.Host.Error)
			}

			result.Git = doctorTool{Name: "git"}
			if path, found := exec.Which("git"); found {
				result.Git.Found = true
				result.Git.Path = path
			} else {
				warnings = append(warnings, "git was not found on PATH")
			}

			result.TestRunners = doctorProbeTools(testRunnerBinaries)
			result.MutationTools = doctorProbeTools(mutationToolBinaries)

			active, activeWarnings, err := doctorProbeActive(f.Home, cfg)
			if err != nil {
				return Outcome{}, err
			}
			result.Active = active
			warnings = append(warnings, activeWarnings...)

			result.LastSessionHost, err = doctorLastSessionHost(f.Home)
			if err != nil {
				return Outcome{}, err
			}

			var hostsWarnings []string
			result.Hosts, hostsWarnings = doctorProbeHosts(f.Home)
			warnings = append(warnings, hostsWarnings...)

			var selftestWarnings []string
			result.HookSelftest, selftestWarnings = doctorProbeHookSelftest()
			warnings = append(warnings, selftestWarnings...)

			var packsWarnings []string
			result.Packs, packsWarnings = doctorProbePacks(f.Home, cfg)
			warnings = append(warnings, packsWarnings...)

			return Outcome{Result: result, Warnings: warnings}, nil
		}),
	}
}

// doctorLoadConfig loads config.json, filling dc without ever
// returning an error: a missing or invalid config is exactly the
// condition doctor exists to report, not to fail on (see the plan's
// design decision 6). Returns the loaded *config.Config, or nil when
// it could not be loaded — every later probe that needs configuration
// treats nil as "skip this check", not as its own error.
func doctorLoadConfig(f Flags, dc *doctorConfig) *config.Config {
	dc.Path = config.ResolvePath(f.Home, f.Config)

	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		var envErr *envelope.Error
		if errors.As(err, &envErr) && envErr.Code == "missing_artifact" {
			dc.Exists = false
		} else {
			dc.Exists = true
			dc.Errors = []string{redactMessage(err)}
		}
		return nil
	}
	dc.Exists = true

	warnings, valErr := config.Validate(cfg, config.DefaultKubeconfigPath())
	dc.Warnings = warnings
	if valErr != nil {
		dc.Errors = flattenJoinedErrors(valErr)
		return cfg
	}
	dc.Valid = true
	return cfg
}

// doctorConfigWarnings folds doctorConfig's own findings into the
// envelope's top-level warnings, so --human output surfaces "config is
// broken" without the reader having to walk into the nested result.
func doctorConfigWarnings(dc doctorConfig) []string {
	var out []string
	if !dc.Exists {
		out = append(out, "config.json does not exist; run `ai-flow config init`")
		return out
	}
	for _, e := range dc.Errors {
		out = append(out, "config: "+e)
	}
	return out
}

// doctorEnvVars reports presence (never value) of every environment
// variable P1's own scope names: Jira's two, and GitHub's token plus
// its two documented fallbacks. KUBECONFIG/AWS_*/ARM_*/Jenkins/eval
// variables belong to later phases (OPS, evals).
func doctorEnvVars() []doctorEnvVar {
	names := []string{"AI_FLOW_JIRA_EMAIL", "AI_FLOW_JIRA_TOKEN", "AI_FLOW_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"}
	out := make([]doctorEnvVar, len(names))
	for i, n := range names {
		out[i] = doctorEnvVar{Name: n, Set: os.Getenv(n) != ""}
	}
	return out
}

// redactMessage passes err's message through redact.New(os.Environ())
// before it ever reaches the result — mirrors ticket.go's redactErr,
// but doctor's errors are plain errors as often as *envelope.Error
// ones (config.Load's decode failures), so this works on the message
// string directly rather than requiring the *envelope.Error type.
func redactMessage(err error) string {
	return redact.New(os.Environ()).String(err.Error())
}

// doctorProbeTracker reports Jira credential presence and, when set,
// attempts Reachable and — only on success — Statuses, comparing the
// result against cfg.Tracker.Statuses (the phase-4 "Decides" line).
// cfg nil (config did not load) skips the statuses comparison, since
// there is nothing configured to compare against.
func doctorProbeTracker(ctx context.Context, cfg *config.Config) doctorTracker {
	email := os.Getenv("AI_FLOW_JIRA_EMAIL")
	token := os.Getenv("AI_FLOW_JIRA_TOKEN")
	dt := doctorTracker{CredsSet: email != "" && token != ""}
	if !dt.CredsSet || cfg == nil {
		return dt
	}

	var trk tracker.Tracker = jira.New(cfg.Tracker, email, token, newJiraTransport(), nil)
	err := trk.Reachable(ctx)
	ok := err == nil
	dt.Reachable = &ok
	if err != nil {
		dt.Error = redactMessage(err)
		return dt
	}

	statuses, err := trk.Statuses(ctx)
	if err != nil {
		dt.Error = redactMessage(err)
		return dt
	}
	dt.MissingStatuses = missingStatusNames(cfg.Tracker.Statuses, statuses)
	return dt
}

// missingStatusNames returns every name in configured (Todo,
// InProgress, InReview) that is not present in real — sorted and
// deduped, so the result is byte-stable regardless of configured
// order.
func missingStatusNames(configured config.TrackerStatuses, real []string) []string {
	var wanted []string
	wanted = append(wanted, configured.Todo...)
	if configured.InProgress != "" {
		wanted = append(wanted, configured.InProgress)
	}
	if configured.InReview != "" {
		wanted = append(wanted, configured.InReview)
	}

	var missing []string
	for _, w := range wanted {
		if !slices.Contains(real, w) {
			missing = append(missing, w)
		}
	}
	sort.Strings(missing)
	return slices.Compact(missing)
}

// doctorProbeHost reports GitHub credential presence (githubToken's own
// fallback precedence — the same one buildHost uses) and, when set,
// attempts Reachable.
func doctorProbeHost(ctx context.Context) doctorHost {
	token := githubToken()
	dh := doctorHost{CredsSet: token != ""}
	if !dh.CredsSet {
		return dh
	}

	var h host.Host = github.New(token, newGitHubTransport(), nil)
	err := h.Reachable(ctx)
	ok := err == nil
	dh.Reachable = &ok
	if err != nil {
		dh.Error = redactMessage(err)
	}
	return dh
}

func doctorProbeTools(names []string) []doctorTool {
	out := make([]doctorTool, len(names))
	for i, n := range names {
		t := doctorTool{Name: n}
		if path, found := exec.Which(n); found {
			t.Found = true
			t.Path = path
		}
		out[i] = t
	}
	return out
}

// doctorProbeActive reports ~/.ai-flow/active.json's own state: absent
// is not an error (no active ticket is a normal state — the same
// convention handoff.LoadActive documents). Stale compares
// LastEventAt against cfg's guard-idle duration when cfg loaded,
// else the documented default (design decision 9).
func doctorProbeActive(home string, cfg *config.Config) (*doctorActive, []string, error) {
	active, err := handoff.LoadActive(home)
	if err != nil {
		return nil, nil, err
	}
	if active == nil {
		return nil, nil, nil
	}

	idle := time.Duration(config.DefaultGuardIdleHours) * time.Hour
	if cfg != nil {
		idle = cfg.Defaults.GuardIdleDuration()
	}

	da := &doctorActive{
		Key:                 active.Key,
		Stage:               active.Stage,
		LastEventAt:         active.LastEventAt,
		DifferentCLIVersion: active.AllowlistHash != exec.Hash(),
	}

	var warnings []string
	if at, err := time.Parse(time.RFC3339, active.LastEventAt); err == nil {
		da.Stale = handoff.Now().Sub(at) > idle
	}
	if da.Stale {
		warnings = append(warnings, "active.json for "+active.Key+" is stale (idle past the guard threshold)")
	}
	if da.DifferentCLIVersion {
		warnings = append(warnings, "active.json for "+active.Key+" was written by a different CLI version (allowlist hash mismatch)")
	}
	return da, warnings, nil
}

// doctorSessionEvent is the tolerant shape doctorLastSessionHost scans
// ~/.ai-flow/events.jsonl for — no writer produces a "host" field
// before phase 9's `hook session-start`, so this always returns nil
// today; it starts reporting the moment that phase lands, with no
// further change here.
type doctorSessionEvent struct {
	Host string `json:"host"`
}

func doctorLastSessionHost(home string) (*string, error) {
	data, err := handoff.ReadArtifact(handoff.GlobalEventsPath(home))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var last string
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var e doctorSessionEvent
		if err := json.Unmarshal(line, &e); err != nil {
			continue // a malformed or unrelated line never fails doctor
		}
		if e.Host != "" {
			last = e.Host
		}
	}
	if last == "" {
		return nil, nil
	}
	return &last, nil
}

// doctorProbeHosts reports each agent host's Detect result — "the
// plugin roots found", phase 9's own line in the growing-surfaces
// registry. A real Detect error (not "not installed here", which is
// (Detection{}, false, nil) by Detect's own contract) becomes a
// warning, never a doctor failure — the plan's design decision 9,
// mirroring every other doctor probe's own "findings are data"
// philosophy (phase 8 design decision 6).
func doctorProbeHosts(home string) ([]doctorHostReport, []string) {
	claudeCode, cursorAdapter := adapters()
	reports := make([]doctorHostReport, 0, 2)
	var warnings []string

	for _, a := range []agenthost.Adapter{claudeCode, cursorAdapter} {
		r := doctorHostReport{Host: string(a.Host())}
		det, found, err := a.Detect(home)
		switch {
		case err != nil:
			r.Error = redactMessage(err)
			warnings = append(warnings, string(a.Host())+": "+r.Error)
		case found:
			r.Detected = true
			r.PluginRoot = det.PluginRoot
			r.Version = det.Version
		}
		reports = append(reports, r)
	}
	return reports, warnings
}

// doctorProbeHookSelftest runs RunHookSelftest for both hosts — the
// same function `hook selftest` itself calls, invoked directly rather
// than shelled out to, exactly like every other doctor probe.
func doctorProbeHookSelftest() ([]doctorSelftestReport, []string) {
	claudeCode, cursorAdapter := adapters()
	reports := make([]doctorSelftestReport, 0, 2)
	var warnings []string

	for _, a := range []agenthost.Adapter{claudeCode, cursorAdapter} {
		host := a.Host()
		result, err := RunHookSelftest(a, host)
		if err != nil {
			warnings = append(warnings, "hook selftest ("+string(host)+"): "+redactMessage(err))
			continue
		}
		reports = append(reports, doctorSelftestReport{Host: string(host), Passed: result.Passed})
		if !result.Passed {
			warnings = append(warnings, "hook selftest ("+string(host)+"): one or more fixtures failed")
		}
	}
	return reports, warnings
}

// doctorProbePacks reports every configured standards source's cache
// state and, when cached, standards.Validate's own result against that
// cache entry — phase 12's "pack block: id, ref, sync state,
// validation result." cfg nil (config did not load) reports nothing,
// same tolerant-of-a-missing-config shape doctorProbeTracker already
// uses. An empty config.standards.sources reports one synthetic row
// for the embedded reference pack, matching config.Validate's own rule
// that an empty list means the embedded pack is the org layer.
func doctorProbePacks(home string, cfg *config.Config) ([]doctorPackReport, []string) {
	if cfg == nil {
		return nil, nil
	}

	sources := standards.SourcesFromConfig(cfg)
	if len(sources) == 0 {
		valid := true
		return []doctorPackReport{{ID: "reference", Layer: "org", Ref: "embedded", Synced: true, Valid: &valid}}, nil
	}

	reports := make([]doctorPackReport, 0, len(sources))
	var warnings []string
	for _, s := range sources {
		r := doctorPackReport{ID: s.ID, Layer: s.Layer, Repo: s.Repo, Ref: s.Ref}

		status := standards.StatusOf(home, s.ID, s.Ref, nil)
		r.Synced = status.Cached
		r.Stale = status.Stale
		if status.RefKind == standards.RefKindBranch && status.Stale {
			warnings = append(warnings, "standards source "+s.ID+": branch ref "+s.Ref+" has not been synced in over a day")
		}

		switch {
		case !status.Cached:
			warnings = append(warnings, "standards source "+s.ID+": not yet synced (run `standards sync`)")
		default:
			_, err := standards.ValidateAgainstProductionAllowlist(standards.CacheDir(home, s.ID, s.Ref))
			valid := err == nil
			r.Valid = &valid
			if err != nil {
				r.Error = redactMessage(err)
				warnings = append(warnings, "standards source "+s.ID+": "+r.Error)
			}
		}
		reports = append(reports, r)
	}
	return reports, warnings
}
