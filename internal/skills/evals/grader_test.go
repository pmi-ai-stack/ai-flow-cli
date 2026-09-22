package evals

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestFirstJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain object", `{"pass": true}`, `{"pass": true}`},
		{"surrounded by prose", `here is my verdict: {"pass": false, "reason": "x"} thanks`, `{"pass": false, "reason": "x"}`},
		{"nested braces", `{"pass": true, "nested": {"a": 1}}`, `{"pass": true, "nested": {"a": 1}}`},
		{"brace inside a string is not structural", `{"reason": "uses a } inside a string", "pass": true}`, `{"reason": "uses a } inside a string", "pass": true}`},
		{"no object at all", `no verdict here`, ""},
		{"unbalanced", `{"pass": true`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstJSONObject(c.in); got != c.want {
				t.Errorf("firstJSONObject(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestGraderSearchText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unwraps a result envelope",
			in:   `{"type":"result","result":"the verdict is {\"pass\": true}"}`,
			want: `the verdict is {"pass": true}`,
		},
		{
			name: "falls back to raw text when there is no result field",
			in:   `{"pass": true}`,
			want: `{"pass": true}`,
		},
		{
			name: "falls back to raw text when it is not JSON at all",
			in:   `{"pass": true} - plain text reply`,
			want: `{"pass": true} - plain text reply`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := graderSearchText(c.in); got != c.want {
				t.Errorf("graderSearchText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestGrade(t *testing.T) {
	root := t.TempDir()
	w, err := NewWorkspace(root, "PROJ-1")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	x := exectest.NewUnlisted(t).Then(exec.Output{
		Stdout:   `{"type":"result","result":"{\"pass\": true, \"matched\": [\"a\"], \"reason\": \"looks right\"}"}`,
		ExitCode: 0,
	})

	s := Scenario{Kind: KindHappyPath, ExpectedBehavior: []string{"a"}}
	got, err := Grade(context.Background(), x, w, "claude-sonnet-5", s, Transcript{Text: "transcript text"})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if !got.Pass || got.Reason != "looks right" {
		t.Errorf("Grade = %+v", got)
	}
}

func TestGrade_UnparseableVerdictFailsWithoutError(t *testing.T) {
	root := t.TempDir()
	w, err := NewWorkspace(root, "PROJ-1")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	x := exectest.NewUnlisted(t).Then(exec.Output{Stdout: "I refuse to answer in JSON", ExitCode: 0})

	got, err := Grade(context.Background(), x, w, "claude-sonnet-5", Scenario{}, Transcript{})
	if err != nil {
		t.Fatalf("Grade should not error on an unparseable verdict: %v", err)
	}
	if got.Pass {
		t.Errorf("Grade.Pass = true, want false for an unparseable verdict")
	}
}
