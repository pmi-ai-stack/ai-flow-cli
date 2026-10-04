// Package runlog is the one place ai-flow writes diagnostic log lines
// (--verbose), always to stderr and never to stdout — stdout carries the
// envelope and nothing else. A Logger travels in the context.Context, so
// nothing about it is package state.
//
// Two formats: text keeps the human lines the clients always wrote; json
// writes one object per line with the run id and command, so a CI log can
// be filtered to one invocation and parsed. Every string is redacted
// before it is encoded.
package runlog

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// Format selects how a line is written.
type Format string

const (
	Text Format = "text"
	JSON Format = "json"
)

// ParseFormat parses --log-format. Empty means Text.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case "", Text:
		return Text, nil
	case JSON:
		return JSON, nil
	}
	return "", &envelope.Error{Code: "usage", Message: fmt.Sprintf("--log-format %q must be \"text\" or \"json\"", s)}
}

// Field is one structured key/value on a JSON line. Text lines ignore
// fields: the message already says it in words.
type Field struct {
	Key   string
	Value any
}

// F builds a Field.
func F(key string, value any) Field { return Field{Key: key, Value: value} }

// NewRunID returns 16 random hex digits naming one invocation.
func NewRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error on supported platforms
	return hex.EncodeToString(b[:])
}

// Logger writes lines. The zero value is not usable; build one with New.
// A nil *Logger is valid and silent, so call sites never check.
type Logger struct {
	mu      sync.Mutex
	w       io.Writer
	format  Format
	runID   string
	command string
	redact  func(string) string
	now     func() time.Time
}

// New builds a Logger. redact is applied to the message and to every
// string field before the line is encoded; now stamps JSON lines (nil
// leaves them without a timestamp — the caller injects the clock, as the
// rest of the module does).
func New(w io.Writer, format Format, runID, command string, redact func(string) string, now func() time.Time) *Logger {
	if redact == nil {
		redact = func(s string) string { return s }
	}
	return &Logger{w: w, format: format, runID: runID, command: command, redact: redact, now: now}
}

// Enabled reports whether l writes anything.
func (l *Logger) Enabled() bool { return l != nil }

// Now is the Logger's injected clock — the zero time for a nil Logger or
// one built without a clock. Callers that time an operation for a log
// line use this rather than reading the wall clock themselves.
func (l *Logger) Now() time.Time {
	if l == nil || l.now == nil {
		return time.Time{}
	}
	return l.now()
}

// Info writes one line.
func (l *Logger) Info(msg string, fields ...Field) {
	if l == nil {
		return
	}
	msg = l.redact(msg)
	var line []byte
	if l.format == JSON {
		rec := map[string]any{
			"level":   "info",
			"run_id":  l.runID,
			"command": l.command,
			"msg":     msg,
		}
		if l.now != nil {
			rec["ts"] = l.now().UTC().Format(time.RFC3339Nano)
		}
		for _, f := range fields {
			rec[f.Key] = l.redactValue(f.Value)
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(rec); err != nil {
			return // a field that cannot be encoded is a caller bug; logging must never fail a command
		}
		line = buf.Bytes()
	} else {
		line = []byte(msg + "\n")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(line)
}

func (l *Logger) redactValue(v any) any {
	switch x := v.(type) {
	case string:
		return l.redact(x)
	case []string:
		out := make([]string, len(x))
		for i, s := range x {
			out[i] = l.redact(s)
		}
		return out
	}
	return v
}

// LineWriter returns an io.Writer that turns every line written to it
// into an Info event tagged with component — the shape the Jira and
// GitHub clients' verbose writer takes. It returns nil for a nil Logger,
// which those clients read as "verbose off".
func (l *Logger) LineWriter(component string) io.Writer {
	if l == nil {
		return nil
	}
	return &lineWriter{l: l, component: component}
}

type lineWriter struct {
	l         *Logger
	component string
	pending   []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.l.Info(string(w.pending[:i]), F("component", w.component))
		w.pending = w.pending[i+1:]
	}
}

type ctxKey struct{}

// WithLogger returns a context carrying l.
func WithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// From returns the Logger ctx carries, or nil (silent) if none.
func From(ctx context.Context) *Logger {
	l, _ := ctx.Value(ctxKey{}).(*Logger)
	return l
}
