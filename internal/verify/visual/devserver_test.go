package visual_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/visual"
)

// fakeUnlistedRunner is a minimal exec.UnlistedRunner test double —
// visual.DevServer never actually needs the process it "starts" to do
// anything, since readiness is polled against a real listener (an
// httptest.Server in these tests) independent of whatever
// StartUnlisted returns.
type fakeUnlistedRunner struct {
	startCalls int
	startErr   error
}

func (f *fakeUnlistedRunner) StartUnlisted(_ context.Context, _, _ string, _, _ []string) (*exec.Process, error) {
	f.startCalls++
	if f.startErr != nil {
		return nil, f.startErr
	}
	// A real Process is required (Stop is a method on *exec.Process,
	// not on the interface) — a short-lived real "true" process is the
	// simplest way to get one without exporting a constructor solely
	// for tests.
	r := exec.NewRunner()
	// "true" is not on the allowlist and StartUnlisted skips it anyway.
	return r.StartUnlisted(context.Background(), ".", "sh", []string{"-c", "sleep 5"}, nil)
}

func (f *fakeUnlistedRunner) RunUnlisted(context.Context, string, string, []string, []string, time.Duration) (exec.Output, error) {
	panic("not used by DevServer")
}

func TestDevServerStartStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	runner := &fakeUnlistedRunner{}
	d := &visual.DevServer{Runner: runner}
	cfg := visual.DevServerConfig{Command: []string{"true"}, URL: srv.URL, StartupTimeoutSeconds: 5}

	if err := d.Start(context.Background(), t.TempDir(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if runner.startCalls != 1 {
		t.Errorf("startCalls = %d, want 1", runner.startCalls)
	}
	if err := d.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestDevServerStartTimeoutStillStops(t *testing.T) {
	// Nothing listens at this address — Start must time out, but still
	// stop the process it started before returning (the plan's own
	// Tests line: "the stop path on failure").
	unused := unusedTCPAddr(t)

	runner := &fakeUnlistedRunner{}
	d := &visual.DevServer{Runner: runner}
	cfg := visual.DevServerConfig{Command: []string{"true"}, URL: "http://" + unused, StartupTimeoutSeconds: 0}
	// StartupTimeoutSeconds 0 exercises pollReady's own timeout path
	// quickly (LoadConfig would default it, but Start takes the config
	// as given) — a fixed short deadline via the config's own field.
	cfg.StartupTimeoutSeconds = 1

	err := d.Start(context.Background(), t.TempDir(), cfg)
	if err == nil {
		t.Fatal("Start: want an error when the dev server never becomes ready")
	}
	// A second Stop call (the caller's own deferred one, on top of
	// Start's internal cleanup) must still be a safe no-op.
	if err := d.Stop(context.Background()); err != nil {
		t.Fatalf("Stop after failed Start: %v, want nil (already stopped)", err)
	}
}

func TestDevServerStopBeforeStartIsNoop(t *testing.T) {
	d := &visual.DevServer{Runner: &fakeUnlistedRunner{}}
	if err := d.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start = %v, want nil", err)
	}
}

func TestDevServerMissingCommand(t *testing.T) {
	d := &visual.DevServer{Runner: &fakeUnlistedRunner{}}
	err := d.Start(context.Background(), t.TempDir(), visual.DevServerConfig{URL: "http://127.0.0.1:1"})
	if err == nil {
		t.Fatal("Start: want an error when no command is configured")
	}
}

func unusedTCPAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}
