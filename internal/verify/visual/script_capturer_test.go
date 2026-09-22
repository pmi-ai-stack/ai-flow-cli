package visual_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/visual"
)

// fakeRunUnlisted is a minimal exec.UnlistedRunner recording the call
// ScriptCapturer.Capture makes and simulating a capture script's
// effect (writing the requested output file) without ever running a
// real subprocess.
type fakeRunUnlisted struct {
	gotTool string
	gotArgs []string
	gotEnv  []string
	write   bool // whether to simulate the script actually writing the file
	fail    error
}

func (f *fakeRunUnlisted) StartUnlisted(context.Context, string, string, []string, []string) (*exec.Process, error) {
	panic("not used by ScriptCapturer")
}

func (f *fakeRunUnlisted) RunUnlisted(_ context.Context, dir, tool string, args, env []string, _ time.Duration) (exec.Output, error) {
	f.gotTool = tool
	f.gotArgs = args
	f.gotEnv = env
	if f.fail != nil {
		return exec.Output{}, f.fail
	}
	if f.write {
		for _, e := range env {
			if out, ok := strings.CutPrefix(e, "AI_FLOW_CAPTURE_OUT="); ok {
				if err := os.WriteFile(out, []byte("fake-png"), 0o600); err != nil {
					panic(err)
				}
			}
		}
	}
	_ = dir
	return exec.Output{}, nil
}

func TestScriptCapturerWritesFile(t *testing.T) {
	fake := &fakeRunUnlisted{write: true}
	c := &visual.ScriptCapturer{Runner: fake, Command: []string{"node", "capture.js"}, Dir: t.TempDir(), Timeout: time.Second}

	out := filepath.Join(t.TempDir(), "screenshot.png")
	res, err := c.Capture(context.Background(), visual.CaptureRequest{URL: "http://localhost:3000/", Viewport: "1440x900", OutputPath: out})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if res.Path != out {
		t.Errorf("Path = %q, want %q", res.Path, out)
	}
	if fake.gotTool != "node" {
		t.Errorf("tool = %q, want node", fake.gotTool)
	}
	wantEnv := []string{"AI_FLOW_CAPTURE_URL=http://localhost:3000/", "AI_FLOW_CAPTURE_VIEWPORT=1440x900", "AI_FLOW_CAPTURE_OUT=" + out}
	for i, want := range wantEnv {
		if i >= len(fake.gotEnv) || fake.gotEnv[i] != want {
			t.Errorf("env[%d] = %v, want %q", i, fake.gotEnv, want)
		}
	}
}

func TestScriptCapturerNoOutputIsToolFailed(t *testing.T) {
	fake := &fakeRunUnlisted{write: false}
	c := &visual.ScriptCapturer{Runner: fake, Command: []string{"node", "capture.js"}, Dir: t.TempDir(), Timeout: time.Second}

	_, err := c.Capture(context.Background(), visual.CaptureRequest{OutputPath: filepath.Join(t.TempDir(), "missing.png")})
	if err == nil {
		t.Fatal("Capture: want an error when the script writes nothing")
	}
}

func TestScriptCapturerNoCommand(t *testing.T) {
	c := &visual.ScriptCapturer{Runner: &fakeRunUnlisted{}}
	_, err := c.Capture(context.Background(), visual.CaptureRequest{})
	if err == nil {
		t.Fatal("Capture: want an error when no command is configured")
	}
}
