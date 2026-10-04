package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/visual"
)

// captureTimeout bounds one screenshot's capture_command run — no
// config surface names a value for it (same reasoning as
// visual.defaultStartupTimeoutSeconds); a fixed, generous default
// stands in.
const captureTimeout = 30 * time.Second

// newUnlistedRunner builds this layer's exec.UnlistedRunner — a
// package-level var, following git.go's newExecer/pr.go's
// newGitHubTransport injectable-constructor pattern, so a test
// substitutes a fake DevServer/ScriptCapturer never spawns a real
// subprocess. Typed as the interface (not *exec.Runner) specifically
// so a fake can satisfy it; production always gets exec.NewRunner(),
// which implements both Execer and UnlistedRunner.
//
// Never call it directly: unlistedRunnerFor is the only caller, so no
// process can leave the allowlist without leaving a line in
// events.jsonl (TestNewUnlistedRunnerOnlyCalledThroughAudit).
var newUnlistedRunner = func() exec.UnlistedRunner { return exec.NewRunner() }

// unlistedRunnerFor is the UnlistedRunner every command uses: each
// process it starts is first recorded as an unlisted_run event in
// eventsPath. The spec's Execution allowlist section allows a process
// outside the table only on that condition.
func unlistedRunnerFor(f Flags, eventsPath string) exec.UnlistedRunner {
	return &auditedUnlisted{inner: newUnlistedRunner(), flags: f, eventsPath: eventsPath}
}

type auditedUnlisted struct {
	inner      exec.UnlistedRunner
	flags      Flags
	eventsPath string
}

func (a *auditedUnlisted) record(tool string, args []string) {
	// Best-effort like every other events.jsonl append: the record is
	// an audit trail, and a full disk must not abort a capture.
	_ = appendEvent(a.flags, a.eventsPath, handoff.UnlistedRunEvent(append([]string{tool}, args...)))
}

func (a *auditedUnlisted) StartUnlisted(ctx context.Context, dir, tool string, args, env []string) (*exec.Process, error) {
	a.record(tool, args)
	return a.inner.StartUnlisted(ctx, dir, tool, args, env)
}

func (a *auditedUnlisted) RunUnlisted(ctx context.Context, dir, tool string, args, env []string, timeout time.Duration) (exec.Output, error) {
	a.record(tool, args)
	return a.inner.RunUnlisted(ctx, dir, tool, args, env, timeout)
}

func newVisualCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "visual",
		Short: "Capture and judge screenshots of the project's dev server",
	}
	cmd.AddCommand(newVisualRunCmd(cio))
	cmd.AddCommand(newVisualJudgeCmd(cio))
	return cmd
}

// visualRunRepoResult names, for one repository, which one visual run
// used — findVisualRepo returns the first repository carrying a
// .ai-flow/visual.json, matching the plan's design decision 11 (no
// --repo flag, same as `e2e run`).
func findVisualRepo(artifact *repoArtifact) (repoArtifactRepository, *visual.Config, bool, error) {
	for _, r := range artifact.Repositories {
		cfg, err := visual.LoadConfig(r.LocalPath)
		if err != nil {
			return repoArtifactRepository{}, nil, false, err
		}
		if cfg != nil {
			return r, cfg, true, nil
		}
	}
	return repoArtifactRepository{}, nil, false, nil
}

type visualRunResult struct {
	Key     string            `json:"key"`
	Repo    string            `json:"repo"`
	Screens map[string]string `json:"screens"` // screen -> screenshot path
}

func newVisualRunCmd(cio cmdIO) *cobra.Command {
	var screenFlags []string
	cmd := &cobra.Command{
		Use:   "run KEY",
		Short: "Start the project's dev server, capture the named (or diff-implied) screens, and stop it",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("visual run", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
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

			repo, visualCfg, ok, err := findVisualRepo(artifact)
			if err != nil {
				return Outcome{}, err
			}
			if !ok {
				return Outcome{}, &envelope.Error{Code: "tool_missing", Message: "no repository carries .ai-flow/visual.json"}
			}

			screens := screenFlags
			if len(screens) == 0 {
				screens = visual.DefaultScreens(artifact.InScopeFiles)
			}

			if f.Simulate {
				return Outcome{Result: visualRunResult{Key: key, Repo: repo.Name, Screens: map[string]string{}}}, nil
			}

			runner := unlistedRunnerFor(f, handoff.TicketEventsPath(f.Home, key))
			dev := &visual.DevServer{Runner: runner}
			if err := dev.Start(cmd.Context(), repo.LocalPath, visualCfg.DevServer); err != nil {
				return Outcome{}, err
			}
			_ = appendEvent(f, handoff.TicketEventsPath(f.Home, key), handoff.Event{
				Type: "dev_server_start", Command: strings.Join(visualCfg.DevServer.Command, " "),
			})
			defer func() {
				_ = dev.Stop(cmd.Context())
				_ = appendEvent(f, handoff.TicketEventsPath(f.Home, key), handoff.Event{
					Type: "dev_server_stop", Command: strings.Join(visualCfg.DevServer.Command, " "),
				})
			}()

			capturer := &visual.ScriptCapturer{Runner: runner, Command: visualCfg.CaptureCommand, Dir: repo.LocalPath, Timeout: captureTimeout}
			screenshotsDir := filepath.Join(ticketDir, "screenshots")
			if err := os.MkdirAll(screenshotsDir, 0o700); err != nil {
				return Outcome{}, fmt.Errorf("cli: create %s: %w", screenshotsDir, err)
			}
			result := visualRunResult{Key: key, Repo: repo.Name, Screens: map[string]string{}}
			for _, screen := range screens {
				outPath := filepath.Join(screenshotsDir, visual.Slug(screen)+".png")
				url := strings.TrimRight(visualCfg.DevServer.URL, "/") + screen
				res, err := captureScreen(cmd.Context(), capturer, url, cfg.Defaults.Viewport, outPath)
				if err != nil {
					return Outcome{}, err
				}
				result.Screens[screen] = res
			}
			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().StringArrayVar(&screenFlags, "screen", nil, "screen path to capture (repeatable); default: the screens the diff implies")
	return cmd
}

// captureScreen captures one screenshot into outPath — the caller has
// already ensured outPath's directory exists (screenshotsDir is the
// same for every screen in one `visual run`, so that only needs doing
// once, not per screen).
func captureScreen(ctx context.Context, c *visual.ScriptCapturer, url, viewport, outPath string) (string, error) {
	res, err := c.Capture(ctx, visual.CaptureRequest{URL: url, Viewport: viewport, OutputPath: outPath})
	if err != nil {
		return "", err
	}
	return res.Path, nil
}

// visualTestResultsScreen mirrors visual-test-results.schema.json's
// screens[] item exactly.
type visualTestResultsScreen struct {
	Screen     string `json:"screen"`
	Verdict    string `json:"verdict"`
	Reason     string `json:"reason"`
	Screenshot string `json:"screenshot,omitempty"`
}

// visualTestResultsArtifact mirrors visual-test-results.schema.json
// exactly.
type visualTestResultsArtifact struct {
	Screens []visualTestResultsScreen `json:"screens"`
}

func loadVisualTestResults(ticketDir string) (visualTestResultsArtifact, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "visual-test-results.json"))
	if err != nil {
		return visualTestResultsArtifact{}, err
	}
	if data == nil {
		return visualTestResultsArtifact{Screens: []visualTestResultsScreen{}}, nil
	}
	var artifact visualTestResultsArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return visualTestResultsArtifact{}, err
	}
	return artifact, nil
}

func newVisualJudgeCmd(cio cmdIO) *cobra.Command {
	var screenFlag, verdictFlag, reasonFlag string
	cmd := &cobra.Command{
		Use:   "judge KEY",
		Short: "Record a verdict for one captured screen",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("visual judge", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			if screenFlag == "" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--screen is required"}
			}
			if verdictFlag != "pass" && verdictFlag != "fail" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--verdict must be pass or fail"}
			}
			if reasonFlag == "" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--reason is required"}
			}
			f := FlagsFrom(cmd.Context())
			ticketDir := handoff.TicketDir(f.Home, key)

			artifact, err := loadVisualTestResults(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			entry := visualTestResultsScreen{Screen: screenFlag, Verdict: verdictFlag, Reason: reasonFlag}
			screenshotPath := filepath.Join(ticketDir, "screenshots", visual.Slug(screenFlag)+".png")
			if _, statErr := os.Stat(screenshotPath); statErr == nil {
				entry.Screenshot = filepath.Join("screenshots", visual.Slug(screenFlag)+".png")
			}

			replaced := false
			for i, s := range artifact.Screens {
				if s.Screen == screenFlag {
					artifact.Screens[i] = entry
					replaced = true
					break
				}
			}
			if !replaced {
				artifact.Screens = append(artifact.Screens, entry)
			}

			if !f.Simulate {
				if _, err := handoff.WriteArtifact(ticketDir, "visual-test-results.json", "visual-test-results", artifact); err != nil {
					return Outcome{}, err
				}
			}
			return Outcome{Result: artifact}, nil
		}),
	}
	cmd.Flags().StringVar(&screenFlag, "screen", "", "the screen being judged (required)")
	cmd.Flags().StringVar(&verdictFlag, "verdict", "", "pass or fail (required)")
	cmd.Flags().StringVar(&reasonFlag, "reason", "", "why (required)")
	return cmd
}
