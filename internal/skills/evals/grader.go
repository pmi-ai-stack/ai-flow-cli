package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// GradeResult is Grade's own verdict on one scenario's transcript.
type GradeResult struct {
	Pass    bool
	Matched []string
	Missing []string
	Reason  string
}

// gradePromptTemplate is the one fixed prompt Grade sends — the
// transcript and the scenario's own expected_behavior[], asking for
// exactly one JSON object on its own line. Kept as a plain format
// string (not a text/template) since its only substitutions are two
// already-plain-text blocks with no conditional structure.
const gradePromptTemplate = `You are grading a transcript of an AI coding assistant session against a fixed list of expected behaviors. Read the transcript and decide, for each expected behavior, whether the transcript demonstrates it.

Expected behaviors:
%s

Transcript:
%s

Respond with exactly one JSON object on its own line, no other text: {"pass": true or false, "matched": [...], "missing": [...], "reason": "..."}. "pass" is true only if every expected behavior is matched.`

// Grade scores t against s.ExpectedBehavior by sending
// gradePromptTemplate through the same headless-Claude-Code path
// claudeCodeRunner itself uses (runClaude) — see the plan's design
// decision 5: the grader is a use of the same host CLI, not a second
// service or credential path. A response with no parseable JSON
// object is GradeResult{Pass: false, ...}, not an error — a bad
// grader response fails the scenario, it does not crash the harness.
func Grade(ctx context.Context, x exec.UnlistedRunner, w *Workspace, model string, s Scenario, t Transcript) (GradeResult, error) {
	prompt := fmt.Sprintf(gradePromptTemplate, bulletList(s.ExpectedBehavior), t.Text)

	verdict, err := runClaude(ctx, x, w, prompt, model)
	if err != nil {
		return GradeResult{}, err
	}

	obj := firstJSONObject(graderSearchText(verdict.Text))
	if obj == "" {
		return GradeResult{Pass: false, Reason: "grader produced no parseable verdict"}, nil
	}

	var parsed struct {
		Pass    bool     `json:"pass"`
		Matched []string `json:"matched"`
		Missing []string `json:"missing"`
		Reason  string   `json:"reason"`
	}
	if err := json.Unmarshal([]byte(obj), &parsed); err != nil {
		return GradeResult{Pass: false, Reason: "grader's verdict did not decode: " + err.Error()}, nil
	}
	return GradeResult{Pass: parsed.Pass, Matched: parsed.Matched, Missing: parsed.Missing, Reason: parsed.Reason}, nil
}

func bulletList(items []string) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString("- ")
		b.WriteString(it)
		b.WriteString("\n")
	}
	return b.String()
}

// graderSearchText resolves what firstJSONObject should scan:
// `claude -p --output-format json`'s own documented shape wraps the
// model's reply in an outer envelope (a top-level "result" string
// field among others not pinned down here — see the plan's §1.3), so
// the model's own {"pass": ...} verdict sits *inside* that string,
// JSON-escaped, not as nested object structure a naive scan of the raw
// bytes would find (the outer envelope itself is the first balanced
// "{...}" span). When text decodes with a non-empty top-level
// "result" string, search inside that; otherwise search text itself,
// so a plain non-JSON-wrapped reply (or a host whose envelope shape
// differs) still has a chance to be found.
func graderSearchText(text string) string {
	var envelope struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(text), &envelope); err == nil && envelope.Result != "" {
		return envelope.Result
	}
	return text
}

// firstJSONObject returns the first balanced {...} span in text, or ""
// if none closes. A headless CLI's --output-format json wraps the
// model's own reply in an outer envelope of its own (a "result" field
// among others) that this package's own contract does not pin down
// (see the plan's §1.3) — scanning for the first balanced object
// inside whatever text comes back is robust to that outer shape
// without this package needing to parse it.
func firstJSONObject(text string) string {
	start := strings.IndexByte(text, '{')
	if start == -1 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		c := text[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// inside a string literal, braces do not count
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}
