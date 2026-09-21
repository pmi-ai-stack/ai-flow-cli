package envelope

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestEnvelopeByteShape asserts the printed envelope is exactly one JSON
// object with two-space indent and a trailing newline, and that field
// order matches the spec's own example (ok, command, result,
// needs_input, error, warnings) rather than an alphabetical sort — the
// envelope's shape is fixed, not data.
func TestEnvelopeByteShape(t *testing.T) {
	e := Ok("version", map[string]any{"version": "dev"})

	var buf bytes.Buffer
	if err := Print(&buf, e); err != nil {
		t.Fatalf("Print: %v", err)
	}
	out := buf.String()

	if !strings.HasSuffix(out, "\n") {
		t.Error("output does not end with a trailing newline")
	}
	if strings.HasSuffix(out, "\n\n") {
		t.Error("output ends with more than one newline")
	}
	if !strings.Contains(out, "\n  \"ok\": true,\n") {
		t.Errorf("output is not two-space indented:\n%s", out)
	}

	// Field order matches the spec's example. Envelope is a Go struct,
	// not a map, so encoding/json already guarantees struct-declaration
	// order for the top level; this just proves it against the actual
	// bytes rather than trusting the guarantee blindly.
	want := []string{`"ok"`, `"command"`, `"result"`, `"needs_input"`, `"error"`, `"warnings"`}
	last := -1
	for _, k := range want {
		idx := strings.Index(out, k)
		if idx == -1 {
			t.Fatalf("output missing key %s:\n%s", k, out)
		}
		if idx <= last {
			t.Errorf("key %s appears out of order (offset %d, previous %d):\n%s", k, idx, last, out)
		}
		last = idx
	}

	// Round-trips as valid, single-object JSON.
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

// TestPrintDoesNotHTMLEscape proves 07-determinism.md's
// SetEscapeHTML(false) rule: a result containing literal "<"/">"/"&"
// (an embedded JSON Schema's description text, a diff, a shell
// snippet) prints those characters as written, not as their JSON
// HTML-escaped </>/& forms — a real bug discovered by
// phase 2's `schema status` output, which embeds "<KEY>" in a
// description field.
func TestPrintDoesNotHTMLEscape(t *testing.T) {
	e := Ok("schema", map[string]any{"description": "~/.ai-flow/<KEY>/status.json & friends"})
	var buf bytes.Buffer
	if err := Print(&buf, e); err != nil {
		t.Fatalf("Print: %v", err)
	}
	out := buf.String()
	forbiddenEscapes := []string{"\\u003c", "\\u003e", "\\u0026"}
	for _, escapeSeq := range forbiddenEscapes {
		if strings.Contains(out, escapeSeq) {
			t.Errorf("Print HTML-escaped the result (found %s):\n%s", escapeSeq, out)
		}
	}
	if !strings.Contains(out, "<KEY>") || !strings.Contains(out, " & friends") {
		t.Errorf("Print output missing literal special characters:\n%s", out)
	}
}

func TestPrintHumanDoesNotHTMLEscape(t *testing.T) {
	e := Ok("schema", map[string]any{"description": "~/.ai-flow/<KEY>/status.json"})
	var buf bytes.Buffer
	if err := PrintHuman(&buf, e); err != nil {
		t.Fatalf("PrintHuman: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "\\u003c") {
		t.Errorf("PrintHuman HTML-escaped the result:\n%s", out)
	}
	if !strings.Contains(out, "<KEY>") {
		t.Errorf("PrintHuman output missing literal special characters:\n%s", out)
	}
}

func TestEnvelopeOkAlwaysHasEmptyWarningsNotNull(t *testing.T) {
	e := Ok("version", nil)
	var buf bytes.Buffer
	if err := Print(&buf, e); err != nil {
		t.Fatalf("Print: %v", err)
	}
	if !strings.Contains(buf.String(), "\"warnings\": []") {
		t.Errorf("warnings should render as [], got:\n%s", buf.String())
	}
}

func TestEnvelopeFailCarriesError(t *testing.T) {
	err := &Error{Code: "usage", Message: "bad flag"}
	e := Fail("version", err)
	if e.OK {
		t.Error("Fail() envelope has OK=true")
	}
	if e.Error == nil || e.Error.Code != "usage" {
		t.Errorf("Fail() envelope.Error = %+v, want code=usage", e.Error)
	}

	var buf bytes.Buffer
	if err := Print(&buf, e); err != nil {
		t.Fatalf("Print: %v", err)
	}
	var decoded struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.OK {
		t.Error("decoded ok=true for a Fail envelope")
	}
	if decoded.Error.Code != "usage" || decoded.Error.Message != "bad flag" {
		t.Errorf("decoded error = %+v", decoded.Error)
	}
}

func TestErrorImplementsGoError(t *testing.T) {
	var err error = &Error{Code: "internal", Message: "boom"}
	if err.Error() != "boom" {
		t.Errorf("Error() = %q, want %q", err.Error(), "boom")
	}
}

func TestPrintHumanOk(t *testing.T) {
	e := Ok("version", map[string]any{"version": "dev"})
	var buf bytes.Buffer
	if err := PrintHuman(&buf, e); err != nil {
		t.Fatalf("PrintHuman: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "version: ok\n") {
		t.Errorf("PrintHuman ok output = %q", out)
	}
}

func TestPrintHumanFail(t *testing.T) {
	e := Fail("version", &Error{Code: "usage", Message: "bad flag"})
	var buf bytes.Buffer
	if err := PrintHuman(&buf, e); err != nil {
		t.Fatalf("PrintHuman: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "bad flag") || !strings.Contains(out, "usage") {
		t.Errorf("PrintHuman fail output = %q", out)
	}
}

func TestPrintHumanNeedsInput(t *testing.T) {
	e := Fail("repo locate", &Error{Code: "routing_ambiguous", Message: "more than one repository matches"})
	e.NeedsInput = &NeedsInput{
		Question:   "Which repository?",
		Options:    []string{"acme.billing.api", "acme.billing.web"},
		ResumeWith: "repo confirm KEY --repos",
	}
	var buf bytes.Buffer
	if err := PrintHuman(&buf, e); err != nil {
		t.Fatalf("PrintHuman: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Which repository?") {
		t.Errorf("PrintHuman needs_input output missing question: %q", out)
	}
	if !strings.Contains(out, "acme.billing.api") {
		t.Errorf("PrintHuman needs_input output missing options: %q", out)
	}
}
