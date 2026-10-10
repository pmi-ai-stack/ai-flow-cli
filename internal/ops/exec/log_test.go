package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/runlog"
)

// logCtx returns a context carrying a JSON logger that redacts through
// rd, and the buffer it writes to.
func logCtx(rd *redact.Redactor) (context.Context, *bytes.Buffer) {
	var buf bytes.Buffer
	l := runlog.New(&buf, runlog.JSON, "run1", "test cmd", rd.String, nil)
	return runlog.WithLogger(context.Background(), l), &buf
}

func logEvents(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %v\n%s", err, line)
		}
		out = append(out, m)
	}
	return out
}

func TestRunLogsStartAndEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	r := testRunner(Row{Tool: "sh", Flags: []string{"-c"}, Class: ClassRepoWrite})
	ctx, buf := logCtx(redact.New(nil))
	dir := t.TempDir()

	if _, err := r.Run(ctx, "sh", []string{"-c", "exit 3"}, RunOpts{Dir: dir}); err == nil {
		t.Fatal("expected a non-zero exit error")
	}
	ev := logEvents(t, buf)
	if len(ev) != 2 {
		t.Fatalf("got %d log events, want exec_start and exec_end:\n%s", len(ev), buf.String())
	}
	if ev[0]["event"] != "exec_start" || ev[0]["tool"] != "sh" || ev[0]["dir"] != dir {
		t.Errorf("start event = %v", ev[0])
	}
	if args, _ := ev[0]["args"].([]any); len(args) != 2 || args[1] != "exit 3" {
		t.Errorf("start args = %v", ev[0]["args"])
	}
	if ev[1]["event"] != "exec_end" || ev[1]["exit_code"] != float64(3) {
		t.Errorf("end event = %v, want exit_code 3", ev[1])
	}
	if _, ok := ev[1]["duration_ms"].(float64); !ok {
		t.Errorf("end event has no numeric duration_ms: %v", ev[1])
	}
	if ev[0]["run_id"] != "run1" || ev[1]["run_id"] != "run1" {
		t.Errorf("run id missing from exec events: %v", ev)
	}
}

func TestRunLogsDenial(t *testing.T) {
	r := testRunner(Row{Tool: "git", Verb: []string{"status"}, Class: ClassRepoWrite})
	ctx, buf := logCtx(redact.New(nil))
	if _, err := r.Run(ctx, "rm", []string{"-rf", "/"}, RunOpts{}); err == nil {
		t.Fatal("expected exec_not_allowed")
	}
	ev := logEvents(t, buf)
	if len(ev) != 1 || ev[0]["event"] != "exec_denied" || ev[0]["tool"] != "rm" {
		t.Fatalf("events = %v, want a single exec_denied for rm", ev)
	}
}

func TestRunLogRedactsSecretsInArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	const secret = "super-secret-token-value"
	rd := redact.New([]string{"AI_FLOW_JIRA_TOKEN=" + secret})
	r := &Runner{Allowlist: NewAllowlist(Row{Tool: "sh", Flags: []string{"-c"}, Class: ClassRepoWrite}), Redact: rd}
	ctx, buf := logCtx(rd)
	if _, err := r.Run(ctx, "sh", []string{"-c", "echo " + secret}, RunOpts{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("the log leaked a secret passed as an argument:\n%s", buf.String())
	}
}

func TestRunWithoutLoggerWritesNothingAndWorks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	r := testRunner(Row{Tool: "echo", Class: ClassRepoWrite})
	if _, err := r.Run(context.Background(), "echo", []string{"hi"}, RunOpts{}); err != nil {
		t.Fatalf("Run without a logger: %v", err)
	}
}

func TestRunUnlistedAndStartUnlistedLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	r := &Runner{Allowlist: NewAllowlist(), Redact: redact.New(nil)}
	ctx, buf := logCtx(redact.New(nil))

	if _, err := r.RunUnlisted(ctx, t.TempDir(), "sh", []string{"-c", "exit 0"}, nil, time.Second); err != nil {
		t.Fatal(err)
	}
	p, err := r.StartUnlisted(ctx, t.TempDir(), "sh", []string{"-c", "sleep 5"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = p.Stop(context.Background(), time.Second)

	var kinds []string
	for _, e := range logEvents(t, buf) {
		kinds = append(kinds, e["event"].(string)+"/"+e["tool"].(string))
	}
	want := []string{"exec_start/sh", "exec_end/sh", "exec_start/sh"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("events = %v, want %v (RunUnlisted start+end, StartUnlisted start)", kinds, want)
	}
}
