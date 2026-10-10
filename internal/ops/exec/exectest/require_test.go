package exectest

import (
	"fmt"
	"testing"
)

// recorder is a testing.TB that records Fatal/Skip/Log instead of
// stopping the goroutine, so RequireTool's three outcomes can be
// observed from outside.
type recorder struct {
	testing.TB
	fatal, skip, log string
}

func (r *recorder) Helper()                        {}
func (r *recorder) Fatalf(format string, a ...any) { r.fatal = fmt.Sprintf(format, a...) }
func (r *recorder) Skipf(format string, a ...any)  { r.skip = fmt.Sprintf(format, a...) }
func (r *recorder) Logf(format string, a ...any)   { r.log = fmt.Sprintf(format, a...) }

const absentTool = "ai-flow-no-such-tool"

func TestRequireToolPresentReturnsPath(t *testing.T) {
	t.Setenv(RequireToolsEnv, "")
	r := &recorder{}
	if path := RequireTool(r, "sh"); path == "" || r.fatal != "" || r.skip != "" {
		t.Errorf("RequireTool(sh) = %q, fatal %q, skip %q; want a path and neither", path, r.fatal, r.skip)
	}
}

func TestRequireToolAbsentSkips(t *testing.T) {
	t.Setenv(RequireToolsEnv, "terraform, kubectl")
	r := &recorder{}
	RequireTool(r, absentTool)
	if r.skip == "" || r.fatal != "" {
		t.Errorf("absent, not required: skip %q, fatal %q; want a skip", r.skip, r.fatal)
	}
}

func TestRequireToolAbsentButRequiredFails(t *testing.T) {
	t.Setenv(RequireToolsEnv, "terraform, "+absentTool+" ,kubectl")
	r := &recorder{}
	RequireTool(r, absentTool)
	if r.fatal == "" || r.skip != "" {
		t.Errorf("absent and required: fatal %q, skip %q; want a failure", r.fatal, r.skip)
	}
}

func TestHasToolOutcomes(t *testing.T) {
	t.Setenv(RequireToolsEnv, "")
	r := &recorder{}
	if !HasTool(r, "sh") {
		t.Error("HasTool(sh) = false")
	}
	if HasTool(r, absentTool) || r.log == "" || r.fatal != "" {
		t.Errorf("absent, not required: log %q, fatal %q; want false and a log line", r.log, r.fatal)
	}

	t.Setenv(RequireToolsEnv, absentTool)
	r = &recorder{}
	if HasTool(r, absentTool) || r.fatal == "" {
		t.Errorf("absent and required: fatal %q; want a failure", r.fatal)
	}
}
