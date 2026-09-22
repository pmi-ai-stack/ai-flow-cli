package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/claudecode"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/cursor"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/guard"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// adapters returns the two production agenthost.Adapter instances
// every hook command picks between. A function, not a package var:
// claudecode.Adapter/cursor.Adapter carry no state, so there is
// nothing here for a test to fake — every test builds its own
// agenthost.Adapter value directly instead.
func adapters() (claudeCode, cursorAdapter agenthost.Adapter) {
	return claudecode.New(), cursor.New()
}

// adapterForHostFlag resolves --host to one of the two adapters, for
// the commands that take it explicitly (hook selftest, hooks install)
// rather than reading it from the environment (hook guard, hook
// session-start — see the plan's design decision, "no --host flag
// anywhere" a host's own hook config invokes).
func adapterForHostFlag(hostFlag string) (agenthost.Adapter, agenthost.Host, error) {
	claudeCode, cursorAdapter := adapters()
	switch agenthost.Host(hostFlag) {
	case agenthost.HostClaudeCode:
		return claudeCode, agenthost.HostClaudeCode, nil
	case agenthost.HostCursor:
		return cursorAdapter, agenthost.HostCursor, nil
	default:
		return nil, "", &envelope.Error{
			Code: "usage",
			Message: fmt.Sprintf("--host must be %q or %q, got %q",
				agenthost.HostClaudeCode, agenthost.HostCursor, hostFlag),
		}
	}
}

func newHookCmd(bi BuildInfo, cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Host hook entry points (guard, session-start) and their diagnostic (selftest)",
	}
	cmd.AddCommand(newHookGuardCmd(cio))
	cmd.AddCommand(newHookSessionStartCmd(bi, cio))
	cmd.AddCommand(newHookSelftestCmd(cio))
	return cmd
}

// --- hook guard ---
//
// Outside the envelope contract (Guardrails: "ai-flow hook <event> ...
// prints a hook response, not an envelope, and uses only two exit
// codes") — this command never calls wrap; it writes the host-shaped
// response itself and sets *cio.exitCode directly.

func newHookGuardCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "guard",
		Short: "Evaluate a PreToolUse hook payload against the guard's protected table",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := FlagsFrom(cmd.Context())
			*cio.exitCode = runHookGuard(cmd.InOrStdin(), cio.stdout, f)
			return nil
		},
	}
}

// runHookGuard is hook guard's whole body, factored out of RunE so it
// is directly unit-testable without going through cobra. Returns 0
// (a decision was printed) or 2 (the payload could not be parsed at
// all — both hosts treat 2 as deny).
func runHookGuard(stdin io.Reader, stdout io.Writer, f Flags) int {
	claudeCode, cursorAdapter := adapters()
	adapter, ok := agenthost.FromEnv(os.Environ(), claudeCode, cursorAdapter)
	if !ok {
		return 2
	}

	data, err := io.ReadAll(stdin)
	if err != nil || len(data) == 0 {
		return 2
	}

	ev, err := adapter.ParseHook(data)
	if err != nil {
		return 2
	}

	// A LoadActive error (a real I/O problem, not "missing" — that's
	// (nil, nil)) is treated exactly like no active ticket: the guard
	// fails open by design, and an unreadable active.json is not this
	// command's business to block a developer's shell command over.
	active, _ := handoff.LoadActive(f.Home)

	decision := guard.Evaluate(guard.Input{
		Event: ev, Active: active, Home: f.Home, Idle: guardIdleDuration(f), Now: handoff.Now(),
	})

	logGuardDecision(f.Home, adapter.Host(), ev, active, decision)

	out, err := adapter.EncodeHook(decision)
	if err != nil {
		return 2
	}
	_, _ = stdout.Write(out)
	_, _ = stdout.Write([]byte("\n"))
	return 0
}

// guardIdleDuration loads config.json tolerantly — a missing or
// invalid config must never make the guard itself fail; it only
// changes which idle threshold applies, falling back to the same
// documented default doctor already uses when config did not load
// (phase 8's own precedent, doctorProbeActive).
func guardIdleDuration(f Flags) time.Duration {
	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return time.Duration(config.DefaultGuardIdleHours) * time.Hour
	}
	return cfg.Defaults.GuardIdleDuration()
}

// logGuardDecision appends a guard_decision event: to the active
// ticket's own events.jsonl when a ticket is active (the only case
// this codebase's singular active.json ever describes — see
// handoff.Active's own doc comment, "There is no per-ticket
// active.json"), else to the global events.jsonl — "guard decisions
// whose path or cwd resolves to no active ticket", the Handoff
// directory section's own words for what belongs there. Logging
// failures are swallowed: a broken events.jsonl write must never turn
// an allow decision into a denial, or vice versa.
func logGuardDecision(home string, host agenthost.Host, ev agenthost.HookEvent, active *handoff.Active, d agenthost.Decision) {
	command := ev.Command
	if command == "" {
		command = ev.FilePath
	}
	event := handoff.GuardDecisionEvent(string(host), d.Reason, command, d.Allow)

	path := handoff.GlobalEventsPath(home)
	if active != nil {
		path = handoff.TicketEventsPath(home, active.Key)
	}
	_ = handoff.AppendEvent(path, event)
}

// --- hook session-start ---

func newHookSessionStartCmd(bi BuildInfo, cio cmdIO) *cobra.Command {
	var pluginRoot string
	cmd := &cobra.Command{
		Use:   "session-start",
		Short: "Create configuration on first run and record a session-start event",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := FlagsFrom(cmd.Context())
			*cio.exitCode = runHookSessionStart(cmd.InOrStdin(), f, bi, pluginRoot)
			return nil
		},
	}
	cmd.Flags().StringVar(&pluginRoot, "plugin-root", "", "the calling host's plugin root")
	return cmd
}

// runHookSessionStart is hook session-start's whole body. Like hook
// guard, it is outside the envelope contract and uses only 0/2. It
// prints nothing to stdout on success — no host-specific SessionStart
// response shape is documented for either host (unlike PreToolUse's),
// so silence is the safe default.
func runHookSessionStart(stdin io.Reader, f Flags, bi BuildInfo, pluginRoot string) int {
	claudeCode, cursorAdapter := adapters()
	adapter, ok := agenthost.FromEnv(os.Environ(), claudeCode, cursorAdapter)
	if !ok {
		return 2
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return 2
	}

	var hostVersion string
	if len(data) > 0 {
		ev, err := adapter.ParseHook(data)
		if err != nil {
			return 2
		}
		hostVersion = ev.HostVersion
	}

	if _, statErr := os.Stat(config.ResolvePath(f.Home, f.Config)); os.IsNotExist(statErr) {
		if _, err := config.Init(f.Home, f.Config); err != nil {
			return 2
		}
	}

	syncAbsentPacks()

	cliVersion := bi.Version
	if cliVersion == "" {
		cliVersion = "dev"
	}
	_ = handoff.AppendEvent(handoff.GlobalEventsPath(f.Home),
		handoff.SessionStartEvent(string(adapter.Host()), hostVersion, pluginRoot, cliVersion))

	return 0
}

// syncAbsentPacks is hook session-start's documented no-op: "syncs
// absent packs once phase 12 exists." internal/standards does not
// exist yet — this is the one call site phase 12 fills a body into,
// not a new call site it has to add.
func syncAbsentPacks() {}

// --- hook selftest ---
//
// Unlike guard/session-start, hook selftest is never wired into a
// host's own hook config — nothing pipes a payload to its stdin — so
// it follows the standard envelope contract like every other
// diagnostic command (the plan's design decision 6).

type hookSelftestFixture struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type hookSelftestResult struct {
	Host     string                `json:"host"`
	Passed   bool                  `json:"passed"`
	Fixtures []hookSelftestFixture `json:"fixtures"`
}

func newHookSelftestCmd(cio cmdIO) *cobra.Command {
	var hostFlag string
	cmd := &cobra.Command{
		Use:   "selftest",
		Short: "Round-trip every embedded hook fixture through parse, guard and encode",
		Args:  cobra.NoArgs,
		RunE: wrap("hook selftest", cio.stdout, cio.exitCode, func(_ *cobra.Command, _ []string) (Outcome, error) {
			adapter, host, err := adapterForHostFlag(hostFlag)
			if err != nil {
				return Outcome{}, err
			}
			result, err := RunHookSelftest(adapter, host)
			if err != nil {
				return Outcome{}, err
			}
			if !result.Passed {
				failed := 0
				for _, fx := range result.Fixtures {
					if !fx.OK {
						failed++
					}
				}
				return Outcome{}, &envelope.Error{
					Code:    "internal",
					Message: fmt.Sprintf("hook selftest: %d/%d fixtures failed for %s", failed, len(result.Fixtures), host),
				}
			}
			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().StringVar(&hostFlag, "host", "", "claude-code or cursor")
	return cmd
}

// RunHookSelftest round-trips every embedded fixture for host through
// ParseHook, guard.Evaluate (for shell/write events) and EncodeHook.
// Exported so doctor (phase 9) calls it directly rather than shelling
// out to itself.
func RunHookSelftest(adapter agenthost.Adapter, host agenthost.Host) (hookSelftestResult, error) {
	names, err := agenthost.FixtureNames(host)
	if err != nil {
		return hookSelftestResult{}, err
	}

	result := hookSelftestResult{Host: string(host), Passed: true}
	for _, name := range names {
		fx := hookSelftestFixture{Name: name, OK: true}
		if err := selftestOne(adapter, host, name); err != nil {
			fx.OK = false
			fx.Detail = err.Error()
			result.Passed = false
		}
		result.Fixtures = append(result.Fixtures, fx)
	}
	return result, nil
}

func selftestOne(adapter agenthost.Adapter, host agenthost.Host, name string) error {
	data, err := agenthost.Fixture(host, name)
	if err != nil {
		return err
	}
	ev, err := adapter.ParseHook(data)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	decision := agenthost.Decision{Allow: true}
	if ev.Event == agenthost.EventShell || ev.Event == agenthost.EventWrite {
		decision = guard.Evaluate(guard.Input{Event: ev, Home: "/selftest", Idle: 8 * time.Hour, Now: time.Unix(0, 0)})
	}
	if _, err := adapter.EncodeHook(decision); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}
