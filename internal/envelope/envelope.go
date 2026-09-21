// Package envelope defines the one JSON object every ai-flow command
// prints on stdout, and the exit-code contract that maps every
// documented error.code to its exit code (0-6).
package envelope

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Envelope is the one JSON object every ai-flow command prints on
// stdout. Field order matches the spec's own example and is preserved
// by encoding/json's struct-field ordering — it is not alphabetically
// sorted, because the envelope's shape is fixed, not data.
type Envelope struct {
	OK         bool        `json:"ok"`
	Command    string      `json:"command"`
	Result     any         `json:"result"`
	NeedsInput *NeedsInput `json:"needs_input"`
	Error      *Error      `json:"error"`
	Warnings   []string    `json:"warnings"`
}

// NeedsInput carries an exit-4 question back to the calling skill, which
// relays needs_input.question, then resumes with resume_with.
type NeedsInput struct {
	Question   string   `json:"question"`
	Options    []string `json:"options"`
	ResumeWith string   `json:"resume_with"`
}

// Error is the envelope's error payload. It implements the error
// interface so a command handler can return *Error directly and have it
// carried through to the envelope without a wrapping type.
type Error struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

// Ok builds a successful envelope for command with the given result.
func Ok(command string, result any) Envelope {
	return Envelope{OK: true, Command: command, Result: result, Warnings: []string{}}
}

// Fail builds a failing envelope for command carrying err.
func Fail(command string, err *Error) Envelope {
	return Envelope{OK: false, Command: command, Error: err, Warnings: []string{}}
}

// Print writes e to w as the one JSON envelope: two-space indent,
// trailing newline, nothing else — stdout is the envelope, and nothing
// but the envelope.
func Print(w io.Writer, e Envelope) error {
	b, err := marshalIndentNoEscape(e)
	if err != nil {
		return fmt.Errorf("envelope: marshal: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("envelope: write: %w", err)
	}
	return nil
}

// marshalIndentNoEscape is json.MarshalIndent with HTML escaping
// disabled — 07-determinism.md requires SetEscapeHTML(false) for
// exactly this reason: content that legitimately contains "<" or ">"
// (an embedded JSON Schema's own description text, a diff, a shell
// snippet in a message) must print as written, not as </>.
// json.MarshalIndent has no such option, so this goes through
// json.Encoder instead and trims the trailing newline Encode always
// appends, to keep MarshalIndent's exact return contract.
func marshalIndentNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// errWriter wraps an io.Writer and records the first write error, so a
// sequence of writes can be expressed as a flat list of calls instead of
// an "if err != nil { return err }" after each one. Once err is set,
// printf is a no-op — there is no point attempting further output to a
// writer that has already failed.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) printf(format string, args ...any) {
	if ew.err != nil {
		return
	}
	_, ew.err = fmt.Fprintf(ew.w, format, args...)
}

// PrintHuman writes a human-readable rendering of e to w, for --human.
// Per-command formatting is added as each command lands; this generic
// form is what every command falls back to.
func PrintHuman(w io.Writer, e Envelope) error {
	ew := &errWriter{w: w}
	if !e.OK {
		if e.Error == nil {
			return fmt.Errorf("envelope: ok=false with no error")
		}
		ew.printf("%s: failed (%s)\n%s\n", e.Command, e.Error.Code, e.Error.Message)
		for _, d := range e.Error.Details {
			ew.printf("  - %s\n", d)
		}
		if e.NeedsInput != nil {
			ew.printf("? %s\n", e.NeedsInput.Question)
			for _, o := range e.NeedsInput.Options {
				ew.printf("  - %s\n", o)
			}
		}
		return ew.err
	}
	ew.printf("%s: ok\n", e.Command)
	if e.Result != nil {
		b, err := marshalIndentNoEscape(e.Result)
		if err != nil {
			return fmt.Errorf("envelope: marshal result: %w", err)
		}
		ew.printf("%s\n", b)
	}
	for _, wmsg := range e.Warnings {
		ew.printf("! %s\n", wmsg)
	}
	return ew.err
}
