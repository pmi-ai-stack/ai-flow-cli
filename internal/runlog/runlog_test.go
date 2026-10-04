package runlog

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

var fixed = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func newTestLogger(f Format, buf *bytes.Buffer) *Logger {
	redact := func(s string) string { return strings.ReplaceAll(s, "s3cr3t", "<redacted>") }
	return New(buf, f, "abc123", "ticket show", redact, func() time.Time { return fixed })
}

func TestTextFormatIsJustTheMessage(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(Text, &buf).Info("GET /rest/api/2/myself", Field{"event", "http"})
	if got := buf.String(); got != "GET /rest/api/2/myself\n" {
		t.Errorf("text line = %q, want the bare message", got)
	}
}

func TestJSONFormatCarriesRunIDCommandAndFields(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(JSON, &buf)
	l.Info("exec: terraform plan", Field{"event", "exec_start"}, Field{"args", []string{"plan", "-no-color"}}, Field{"exit_code", 0})
	l.Info("second")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), buf.String())
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line is not JSON: %v\n%s", err, lines[0])
	}
	for k, want := range map[string]any{
		"run_id": "abc123", "command": "ticket show", "level": "info", "msg": "exec: terraform plan",
		"event": "exec_start", "ts": "2026-10-04T12:00:00Z", "exit_code": float64(0),
	} {
		if first[k] != want {
			t.Errorf("%s = %v, want %v", k, first[k], want)
		}
	}
	if args, _ := first["args"].([]any); len(args) != 2 || args[0] != "plan" {
		t.Errorf("args = %v", first["args"])
	}
	var second map[string]any
	_ = json.Unmarshal([]byte(lines[1]), &second)
	if second["run_id"] != "abc123" {
		t.Errorf("run id differs between lines of one run: %v", second["run_id"])
	}
}

// A secret is redacted before encoding: one that JSON would escape
// (a quote, a backslash) must not survive in a different spelling.
func TestRedactsMessageAndEveryStringField(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(JSON, &buf)
	l.Info("token s3cr3t used", Field{"dir", "/x/s3cr3t"}, Field{"args", []string{"--token=s3cr3t", "ok"}})
	if strings.Contains(buf.String(), "s3cr3t") {
		t.Fatalf("a secret reached the log:\n%s", buf.String())
	}
	buf.Reset()
	newTestLogger(Text, &buf).Info("token s3cr3t")
	if strings.Contains(buf.String(), "s3cr3t") {
		t.Fatalf("a secret reached the text log:\n%s", buf.String())
	}
}

func TestNilLoggerIsANoOp(t *testing.T) {
	var l *Logger
	l.Info("nothing", Field{"k", "v"})
	if l.Enabled() {
		t.Error("nil logger reports enabled")
	}
	if w := l.LineWriter("jira"); w != nil {
		t.Error("nil logger returned a non-nil LineWriter")
	}
	if From(context.Background()).Enabled() {
		t.Error("a context without a logger yields an enabled one")
	}
}

func TestLineWriterTurnsEachLineIntoAnEvent(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(JSON, &buf)
	w := l.LineWriter("jira")
	if _, err := w.Write([]byte("GET /a\nGET /b\n")); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d events for 2 written lines:\n%s", len(lines), buf.String())
	}
	var e map[string]any
	_ = json.Unmarshal([]byte(lines[1]), &e)
	if e["msg"] != "GET /b" || e["component"] != "jira" {
		t.Errorf("event = %v, want msg GET /b from component jira", e)
	}
}

func TestContextRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(Text, &buf)
	if From(WithLogger(context.Background(), l)) != l {
		t.Error("From did not return the logger WithLogger stored")
	}
}

func TestParseFormat(t *testing.T) {
	for in, want := range map[string]Format{"": Text, "text": Text, "json": JSON} {
		if got, err := ParseFormat(in); err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat(yaml) accepted")
	}
}

func TestNewRunID(t *testing.T) {
	a, b := NewRunID(), NewRunID()
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(a) || a == b {
		t.Errorf("run ids %q, %q: want two distinct 16-hex-digit ids", a, b)
	}
}

func TestConcurrentLinesDoNotInterleave(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(JSON, &buf)
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() { l.Info("line", Field{"n", i}); done <- struct{}{} }()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("interleaved or torn line %q: %v", line, err)
		}
	}
}

func TestNoClockMeansNoTimestampAndZeroNow(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, JSON, "r", "c", nil, nil)
	l.Info("x")
	if strings.Contains(buf.String(), `"ts"`) {
		t.Errorf("a logger without a clock wrote a timestamp: %s", buf.String())
	}
	if !l.Now().IsZero() {
		t.Error("Now() without a clock is not the zero time")
	}
	var nilLogger *Logger
	if !nilLogger.Now().IsZero() {
		t.Error("nil Logger.Now() is not the zero time")
	}
	if got := newTestLogger(JSON, &buf).Now(); !got.Equal(fixed) {
		t.Errorf("Now() = %v, want the injected %v", got, fixed)
	}
}
