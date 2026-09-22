//go:build integration

package visual_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/visual"
)

// TestScriptCapturerRealScript proves the full env-variable contract
// end to end against a real subprocess (not a fake) — a tiny shell
// script that reads AI_FLOW_CAPTURE_{URL,VIEWPORT,OUT} and writes a
// one-line "screenshot" file, exactly the contract a real
// project-provided capture script is expected to honour.
func TestScriptCapturerRealScript(t *testing.T) {
	script := filepath.Join(t.TempDir(), "capture.sh")
	body := "#!/bin/sh\nprintf '%s %s\\n' \"$AI_FLOW_CAPTURE_URL\" \"$AI_FLOW_CAPTURE_VIEWPORT\" > \"$AI_FLOW_CAPTURE_OUT\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := &visual.ScriptCapturer{
		Runner:  exec.NewRunner(),
		Command: []string{"sh", script},
		Dir:     t.TempDir(),
		Timeout: 5 * time.Second,
	}
	out := filepath.Join(t.TempDir(), "screenshot.txt")

	res, err := c.Capture(context.Background(), visual.CaptureRequest{URL: "http://localhost:4000/dashboard", Viewport: "1440x900", OutputPath: out})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if res.Path != out {
		t.Errorf("Path = %q, want %q", res.Path, out)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "http://localhost:4000/dashboard 1440x900\n"
	if string(data) != want {
		t.Errorf("output = %q, want %q", data, want)
	}
}
