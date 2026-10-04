package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/host"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/host/github"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/harness"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
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

// doctorGuidanceBudget is the active ticket's guidance-budget health —
// phase 13's own doctor line ("doctor reports it" for the must-overflow
// case). Per ticket rather than global, because the budget is only ever
// exceeded by a *resolution*, and a resolution belongs to a ticket;
// the active ticket is the only one doctor already knows about.
type doctorGuidanceBudget struct {
	Key    string                      `json:"key"`
	Tokens int                         `json:"tokens"`
	Stages []doctorGuidanceBudgetStage `json:"stages"`
}

// doctorGuidanceBudgetStage is one stage whose must sections alone
// overflowed the budget. Stages that fit are not listed: doctor reports
// findings, not an inventory.
type doctorGuidanceBudgetStage struct {
	Stage             string `json:"stage"`
	MustOverflowBytes int    `json:"must_overflow_bytes"`
	Dropped           int    `json:"dropped"`
}

type doctorResult struct {
	Config          doctorConfig           `json:"config"`
	EnvVars         []doctorEnvVar         `json:"env_vars"`
	Tracker         doctorTracker          `json:"tracker"`
	Host            doctorHost             `json:"host"`
	Git             doctorTool             `json:"git"`
	TestRunners     []doctorTool           `json:"test_runners"`
	MutationTools   []doctorTool           `json:"mutation_tools"`
	AddonBinaries   []doctorTool           `json:"addon_binaries"`
	Active          *doctorActive          `json:"active"`
	LastSessionHost *string                `json:"last_session_host"`
	Hosts           []doctorHostReport     `json:"hosts"`
	HookSelftest    []doctorSelftestReport `json:"hook_selftest"`
	Packs           []doctorPackReport     `json:"packs"`
	GuidanceBudget  *doctorGuidanceBudget  `json:"guidance_budget"`

	// ExpiringWaivers is the active ticket's repositories' waivers that
	// are already expired or lapse within waiverExpiringDays. An expired
	// waiver has already stopped suppressing its check; this is the
	// advance notice for the rest.
	ExpiringWaivers []doctorWaiver `json:"expiring_waivers"`

	// MissingCheckRunners is the phase-14 growing-surfaces line: every
	// tool the active ticket's resolved checks name that is not on
	// PATH. Populated only for a real, effective (non-overridden)
	// check with a Tool at all — dependency/layout/docs/commit/pr are
	// built-in and never appear here.
	MissingCheckRunners []doctorMissingCheckRunner `json:"missing_check_runners"`

	// LiveEnvironments is the phase-20 growing-surfaces "live probes"
	// line: config.ops.environments's own kube-bound entries, probed
	// with "kubectl auth can-i --list" (spec's own literal example).
	LiveEnvironments []doctorLiveEnvironment `json:"live_environments"`

	// LeakedInfraTestClusters is the phase-24 growing-surfaces "kind
	// clusters left by a crashed run" line: every kind cluster whose
	// name matches the ticket-key-derived pattern
	// (internal/ops/harness.ClusterName's own lowercased-key shape) but
	// is not the currently active ticket's own cluster — see
	// doctorProbeInfraTestClusters.
	LeakedInfraTestClusters []string `json:"leaked_infra_test_clusters"`

	// RenderCache is the phase-30 growing-surfaces "doctor" line: entry
	// count, how many are older than render.DefaultCacheMaxAge, and when
	// hook session-start last actually pruned it.
	RenderCache doctorRenderCache `json:"render_cache"`
}

// doctorLiveEnvironment is one config.ops.environments entry's live
// read-only probe result. Only environments with a Kube binding are
// probed: no other addon-independent, credential-only read-only check
// exists yet before an addon (terraform/jenkins land in 21/27) can
// answer the same question through its own Drift/DryRun call.
type doctorLiveEnvironment struct {
	Environment string `json:"environment"`
	Tier        string `json:"tier"`
	Context     string `json:"context"`
	Reachable   *bool  `json:"reachable"`
	// ReadOnly is a provisional heuristic (no mutating verb keyword
	// found in "kubectl auth can-i --list"'s own stdout) — kubectl has
	// no "-o json" form of can-i --list as of the versions this spec
	// targets, and a positional-column parser against an unversioned,
	// human-oriented table for a doctor *warning* (never a hard
	// failure) is not proportionate. Flagged explicitly here, the same
	// posture phase 9 already established for its own guessed Cursor
	// payload shapes. nil when Reachable is false — there is no output
	// to read a verdict from.
	ReadOnly *bool  `json:"read_only"`
	Error    string `json:"error,omitempty"`
}

// doctorMissingCheckRunner is one check whose tool doctor could not
// find on PATH.
type doctorMissingCheckRunner struct {
	CheckID string `json:"check_id"`
	Tool    string `json:"tool"`
}

// testRunnerBinaries and mutationToolBinaries are the fixed tables
// doctor probes with exec.Which — the same binaries
// internal/detect.Runners/Mutation name for the four language
// ecosystems this codebase supports today. doctor is not ticket-scoped
// (no repo.json to detect against), so a fixed table is the only
// detection-free reading of "test and mutation runners" available.
var testRunnerBinaries = []string{"go", "npx", "pytest"}
var mutationToolBinaries = []string{"gremlins", "npx", "mutmut", "mvn"}

// addonToolBinaries is the phase-18 growing-surfaces "addon binaries"
// line: every tool name this phase's own internal/ops/exec.Table rows
// name, deduped, so `doctor` reports which of them are missing before
// a real Addon (phase 21 onward) ever tries to run one. terraform's
// `tflint`, kubectl's `kubeconform`, helm's `ct`, and the two purely
// local tools (kustomize, actionlint, act) are all named here even
// though no addon calls them yet — matching table.go's own "the
// allowlist is data, decoupled from any Go type" reasoning.
var addonToolBinaries = []string{
	"terraform", "tflint", "kubectl", "kubeconform", "kuttl", "chainsaw",
	"helm", "ct", "kustomize", "actionlint", "act", "jenkinsfile-runner",
	// Phase 24's own addition: the per-ticket kind cluster's own tool.
	// A plain slice element, never a call argument, so this has no
	// effect on internal/ops/exec's own reachability scanner.
	"kind",
}

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

			result.Host = doctorProbeHost(cmd.Context(), cfg)
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
			result.AddonBinaries = doctorProbeTools(addonToolBinaries)

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

			// Read once, not once per probe: doctorProbeGuidanceBudget and
			// doctorProbeCheckRunners both read the same active ticket's
			// standards.json.
			activeStandards := doctorActiveStandards(f.Home, result.Active)

			var waiverWarnings []string
			result.ExpiringWaivers, waiverWarnings = doctorProbeWaivers(f.Home)
			warnings = append(warnings, waiverWarnings...)

			var budgetWarnings []string
			result.GuidanceBudget, budgetWarnings = doctorProbeGuidanceBudget(result.Active, activeStandards)
			warnings = append(warnings, budgetWarnings...)

			var checkRunnerWarnings []string
			result.MissingCheckRunners, checkRunnerWarnings = doctorProbeCheckRunners(result.Active, activeStandards)
			warnings = append(warnings, checkRunnerWarnings...)

			var liveWarnings []string
			result.LiveEnvironments, liveWarnings = doctorProbeLiveEnvironments(cmd.Context(), cfg)
			warnings = append(warnings, liveWarnings...)

			var clusterWarnings []string
			result.LeakedInfraTestClusters, clusterWarnings = doctorProbeInfraTestClusters(cmd.Context(), result.Active)
			warnings = append(warnings, clusterWarnings...)

			var cacheWarnings []string
			result.RenderCache, cacheWarnings = doctorProbeRenderCache(f.Home)
			warnings = append(warnings, cacheWarnings...)

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
		if isMissingArtifact(err) {
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
// attempts Reachable and — only on success — Statuses, each request
// bounded by config.defaults.http_timeout_seconds, comparing the
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

	timeout := cfg.Defaults.HTTPTimeout()
	ctx, cancel := context.WithTimeout(ctx, 2*timeout) // one deadline over both calls below, each also bounded by the client
	defer cancel()
	var trk tracker.Tracker = jira.New(cfg.Tracker, email, token, newJiraTransport(), nil).WithTimeout(timeout)
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
// attempts Reachable under config.defaults.http_timeout_seconds (the
// documented default when cfg is nil — config did not load), so an
// unreachable host is a reachable:false finding, never a hung doctor.
func doctorProbeHost(ctx context.Context, cfg *config.Config) doctorHost {
	token := githubToken()
	dh := doctorHost{CredsSet: token != ""}
	if !dh.CredsSet {
		return dh
	}

	timeout := config.Defaults{}.HTTPTimeout()
	if cfg != nil {
		timeout = cfg.Defaults.HTTPTimeout()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var h host.Host = github.New(token, newGitHubTransport(), nil).WithTimeout(timeout)
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

// doctorActiveStandards reads the active ticket's standards.json once,
// shared by doctorProbeGuidanceBudget and doctorProbeCheckRunners —
// both used to read it independently, doubling the same file read and
// JSON decode on every `doctor` run. No active ticket, no
// standards.json yet, or one this binary cannot parse all report nil,
// exactly as each probe's own prior direct read did.
func doctorActiveStandards(home string, active *doctorActive) *standards.Artifact {
	if active == nil {
		return nil
	}
	a, err := standards.ReadArtifact(handoff.TicketDir(home, active.Key))
	if err != nil {
		return nil
	}
	return a
}

// doctorProbeGuidanceBudget reports every stage whose `must` sections
// alone exceeded config.standards.guidance_budget_tokens — the spec's
// "guidance.md still carries every must section, records `Budget
// exceeded by must sections:` with the overflow in bytes, and doctor
// reports it."
//
// No active ticket or no standards.json yet reports nothing: doctor's
// own philosophy is findings-as-data and never failing, and "there is
// nothing resolved to check" is not a finding.
func doctorProbeGuidanceBudget(active *doctorActive, a *standards.Artifact) (*doctorGuidanceBudget, []string) {
	if active == nil || a == nil {
		return nil, nil
	}

	report := &doctorGuidanceBudget{Key: active.Key, Tokens: a.Budget.Tokens, Stages: []doctorGuidanceBudgetStage{}}
	var warnings []string
	for _, ps := range a.Budget.PerStage {
		if ps.MustOverflowBytes == 0 {
			continue
		}
		report.Stages = append(report.Stages, doctorGuidanceBudgetStage{
			Stage: ps.Stage, MustOverflowBytes: ps.MustOverflowBytes, Dropped: len(ps.Dropped),
		})
		warnings = append(warnings, fmt.Sprintf(
			"%s: the %s stage's must sections exceed guidance_budget_tokens by %d bytes; nothing is truncated, but every should and may is dropped",
			active.Key, ps.Stage, ps.MustOverflowBytes))
	}
	return report, warnings
}

// whichOverride is exec.Which by default; tests reassign it so
// doctorProbeCheckRunners' own logic can be proven without depending
// on which check-runner binaries happen to be on the machine running
// the suite — the same injectable-constructor pattern git.go's
// newExecer and pr.go's newGitHubTransport already use.
var whichOverride = exec.Which

// doctorProbeCheckRunners reports every distinct tool an effective
// check names that is not on PATH — the phase-14 plan's own "doctor
// gains missing check runners" growing-surfaces line. Reads the
// already-resolved standards.json (via doctorActiveStandards) rather
// than re-resolving (no repo checkout access is guaranteed from
// wherever `doctor` runs), so this is silent whenever there is no
// active ticket or no standards.json yet — the same "nothing resolved
// to check is not a finding" philosophy doctorProbeGuidanceBudget
// already states.
func doctorProbeCheckRunners(active *doctorActive, a *standards.Artifact) ([]doctorMissingCheckRunner, []string) {
	if active == nil || a == nil {
		return nil, nil
	}

	var missing []doctorMissingCheckRunner
	var warnings []string
	seen := map[string]bool{}
	for _, c := range a.Checks {
		if c.Tool == "" || c.OverriddenBy != nil || seen[c.Tool] {
			continue
		}
		seen[c.Tool] = true
		if _, found := whichOverride(c.Tool); !found {
			missing = append(missing, doctorMissingCheckRunner{CheckID: c.ID, Tool: c.Tool})
			warnings = append(warnings, fmt.Sprintf("check %s: tool %q was not found on PATH", c.ID, c.Tool))
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].CheckID < missing[j].CheckID })
	return missing, warnings
}

// mutatingCanIVerbs is the keyword set doctorProbeLiveEnvironments's
// own provisional read_only heuristic (see doctorLiveEnvironment's doc
// comment) treats as evidence the configured context can mutate live
// state.
var mutatingCanIVerbs = []string{"create", "update", "patch", "delete", "deletecollection"}

// doctorProbeLiveEnvironments runs the spec's own documented live
// read-only probe ("kubectl auth can-i --list" → "read_only") against
// every config.ops.environments entry with a Kube binding — the one
// addon-independent, credential-only signal a doctor probe can check
// before any real addon (kubectl lands in phase 22) exists to ask
// through Addon.Drift/DryRun itself. cfg nil (config did not load)
// reports nothing, the same tolerant-of-a-missing-config shape every
// other cfg-dependent probe in this file already uses. A probe failure
// (kubectl absent, cluster unreachable) is doctor's own kind of
// finding — reported as data, never returned as a command error.
func doctorProbeLiveEnvironments(ctx context.Context, cfg *config.Config) ([]doctorLiveEnvironment, []string) {
	if cfg == nil {
		return nil, nil
	}
	names := make([]string, 0, len(cfg.Ops.Environments))
	for name, e := range cfg.Ops.Environments {
		if e.Kube != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, nil
	}

	execer := newExecer()
	var out []doctorLiveEnvironment
	var warnings []string
	for _, name := range names {
		env := cfg.Ops.Environments[name]
		probe := doctorLiveEnvironment{Environment: name, Tier: env.Tier, Context: env.Kube.Context}

		result, err := execer.Run(ctx, "kubectl", []string{"auth", "can-i", "--list", "--context", env.Kube.Context}, exec.RunOpts{Timeout: cfg.Ops.ToolTimeout()})
		ok := err == nil
		probe.Reachable = &ok
		if err != nil {
			probe.Error = redactMessage(err)
			warnings = append(warnings, fmt.Sprintf("ops: environment %q: live probe failed: %s", name, probe.Error))
			out = append(out, probe)
			continue
		}

		readOnly := !containsAny(result.Stdout, mutatingCanIVerbs)
		probe.ReadOnly = &readOnly
		if !readOnly {
			warnings = append(warnings, fmt.Sprintf("ops: environment %q: configured kube context can mutate live state; use a read-only account", name))
		}
		out = append(out, probe)
	}
	return out, warnings
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// isTicketClusterName reports whether n is shaped like
// harness.ClusterName's own output (a lowercased ticket key) — reusing
// handoff.ValidateKey's own canonical pattern (case-inverted, since
// kind's own cluster names are lowercase) rather than hand-maintaining
// a second regexp that would need to be kept in sync by hand — the
// pattern doctorProbeInfraTestClusters uses to tell an ai-flow-owned
// cluster apart from one that belongs to something else entirely on
// the same machine.
func isTicketClusterName(n string) bool {
	return handoff.ValidateKey(strings.ToUpper(n)) == nil
}

// doctorProbeInfraTestClusters is the phase-24 growing-surfaces "kind
// clusters left by a crashed run" line: every kind cluster whose name
// looks like an ai-flow ticket's own (isTicketClusterName) but is not
// the currently active ticket's own cluster is reported as a leak —
// the doctor-philosophy "report as data, never fail" shape every other
// probe in this file already uses. Calls
// internal/ops/harness.ListClusters rather than the "kind" tool
// directly, so this file's own literal "kind" call-argument count
// stays zero (see internal/ops/exec/reachability_test.go's own allowed
// list, which does not name doctor.go). A missing kind binary reports
// no leaks at all — ListClusters' own tolerant contract, identical to
// how TeardownCluster treats it.
func doctorProbeInfraTestClusters(ctx context.Context, active *doctorActive) ([]string, []string) {
	names, err := harness.ListClusters(ctx, newExecer())
	if err != nil {
		return nil, []string{"ops: listing kind clusters: " + redactMessage(err)}
	}
	activeCluster := ""
	if active != nil {
		activeCluster = harness.ClusterName(active.Key)
	}

	var leaked []string
	for _, n := range names {
		if isTicketClusterName(n) && n != activeCluster {
			leaked = append(leaked, n)
		}
	}
	sort.Strings(leaked)

	var warnings []string
	for _, n := range leaked {
		warnings = append(warnings, fmt.Sprintf("ops: kind cluster %q looks like a leaked ai-flow infra-test cluster; run `ai-flow ops test teardown` for the ticket it belongs to, or delete it by hand", n))
	}
	return leaked, warnings
}

// doctorRenderCache is the phase-30 growing-surfaces "doctor" line:
// visibility into ~/.ai-flow/cache/render/, which hook session-start
// now prunes against render.DefaultCacheMaxAge (see hook.go's own
// pruneStaleRenderCacheIfDue). A doctor run never prunes anything
// itself — it only reports what the next session-start would do.
type doctorRenderCache struct {
	Entries      int     `json:"entries"`
	StaleEntries int     `json:"stale_entries"`
	LastPrunedAt *string `json:"last_pruned_at"`
}

// doctorProbeRenderCache reports the cache's own size and staleness,
// never an error: a missing cache/render/ directory (nothing rendered
// yet) is zero entries, not a finding.
func doctorProbeRenderCache(home string) (doctorRenderCache, []string) {
	var result doctorRenderCache

	entries, err := render.CacheEntries(home)
	if err != nil {
		return result, []string{"render cache: " + redactMessage(err)}
	}
	result.Entries = len(entries)

	now := handoff.Now()
	for _, e := range entries {
		if now.Sub(e.ModTime) >= render.DefaultCacheMaxAge {
			result.StaleEntries++
		}
	}

	if info, statErr := os.Stat(renderCachePrunedAtPath(home)); statErr == nil {
		ts := info.ModTime().UTC().Format(time.RFC3339)
		result.LastPrunedAt = &ts
	}

	var warnings []string
	if result.StaleEntries > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"render cache: %d of %d entries are older than %s and will be pruned at the next session start",
			result.StaleEntries, result.Entries, render.DefaultCacheMaxAge))
	}
	return result, warnings
}

// doctorWaiver is one waiver doctor flags: repo is the repository
// directory's name, as `check`'s own findings name it.
type doctorWaiver struct {
	Repo     string `json:"repo"`
	CheckID  string `json:"check_id"`
	Expires  string `json:"expires"`
	State    string `json:"state"`
	DaysLeft int    `json:"days_left"`
}

// doctorProbeWaivers reports expired and soon-to-expire waivers in the
// active ticket's repositories — the only repositories doctor knows. A
// repository whose waivers file cannot be read is skipped, never an
// error: doctor reports on what it can read.
func doctorProbeWaivers(home string) ([]doctorWaiver, []string) {
	out := []doctorWaiver{}
	active, err := handoff.LoadActive(home)
	if err != nil || active == nil {
		return out, nil
	}
	var warnings []string
	now := handoff.Now()
	window := time.Duration(waiverExpiringDays) * 24 * time.Hour
	for _, root := range active.RepoRoots {
		repo := filepath.Base(root)
		ws, err := standards.LoadWaivers(repo, root)
		if err != nil {
			continue
		}
		for _, w := range ws {
			state, days := standards.ExpiryState(w, now, window)
			switch state {
			case standards.StateExpired:
				warnings = append(warnings, fmt.Sprintf("waivers: %s: waiver for check %s expired on %s; the check fires again until it is fixed or re-waived", repo, w.CheckID, w.Expires))
			case standards.StateExpiring:
				warnings = append(warnings, fmt.Sprintf("waivers: %s: waiver for check %s expires on %s (in %d days)", repo, w.CheckID, w.Expires, days))
			default:
				continue
			}
			out = append(out, doctorWaiver{Repo: repo, CheckID: w.CheckID, Expires: w.Expires, State: state, DaysLeft: days})
		}
	}
	return out, warnings
}
