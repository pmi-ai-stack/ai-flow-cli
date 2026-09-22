package visual

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// stopGrace bounds Stop's wait between SIGTERM and SIGKILL — a
// generous, fixed value; no config surface names one (same reasoning
// as config.go's defaultStartupTimeoutSeconds).
const stopGrace = 5 * time.Second

// pollInterval is how often DevServer.Start retries pollReady.
const pollInterval = 100 * time.Millisecond

// DevServer starts a repository's dev server, waits for it to accept
// connections, and stops it again. Runner is exec.UnlistedRunner (an
// interface, not *exec.Runner) so a test injects a fake that never
// spawns a real process.
type DevServer struct {
	Runner exec.UnlistedRunner
	proc   *exec.Process
}

// Start runs cfg.Command in dir and blocks until cfg.URL accepts a TCP
// connection or cfg.StartupTimeoutSeconds elapses. On a timeout (or
// any other failure after the process started), the process is still
// stopped before Start returns — a caller's own Stop call afterward is
// then a safe no-op (Process.Stop's own idempotency) rather than a
// leaked process, matching the plan's Tests line ("the stop path on
// failure").
func (d *DevServer) Start(ctx context.Context, dir string, cfg DevServerConfig) error {
	if len(cfg.Command) == 0 {
		return &envelope.Error{Code: "tool_missing", Message: "no dev_server.command configured in .ai-flow/visual.json"}
	}
	proc, err := d.Runner.StartUnlisted(ctx, dir, cfg.Command[0], cfg.Command[1:], nil)
	if err != nil {
		return fmt.Errorf("visual: start dev server: %w", err)
	}
	d.proc = proc

	timeout := time.Duration(cfg.StartupTimeoutSeconds) * time.Second
	if err := pollReady(ctx, cfg.URL, timeout); err != nil {
		_ = proc.Stop(ctx, stopGrace)
		return err
	}
	return nil
}

// Stop terminates the dev server Start started. Safe to call when
// Start was never called or failed before starting a process
// (Process.Stop's own nil-safety) and safe to call twice.
func (d *DevServer) Stop(ctx context.Context) error {
	return d.proc.Stop(ctx, stopGrace)
}

// pollReady polls url's host:port with a short-lived TCP dial until
// one succeeds or timeout elapses.
func pollReady(ctx context.Context, rawURL string, timeout time.Duration) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("visual: parse dev_server.url %q: %w", rawURL, err)
	}
	addr := u.Host

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var dialer net.Dialer
	for {
		conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
		if dialErr == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return &envelope.Error{
				Code:    "tool_failed",
				Message: fmt.Sprintf("dev server did not become ready at %s within %s", addr, timeout),
			}
		case <-time.After(pollInterval):
		}
	}
}
