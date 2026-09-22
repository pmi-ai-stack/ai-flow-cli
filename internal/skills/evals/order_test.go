package evals

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpectedCommandOrder(t *testing.T) {
	cases := []struct {
		name     string
		behavior []string
		want     []string
	}{
		{
			name:     "develop happy path",
			behavior: []string{"runs ai-flow status KEY", "loads the named skill"},
			want:     []string{"status"},
		},
		{
			name:     "develop temptation",
			behavior: []string{"refuses to bypass the gate", "runs stage approve instead"},
			want:     []string{"stage approve"},
		},
		{
			name:     "run-mutation happy path",
			behavior: []string{"runs mutation run", "runs mutation summarize"},
			want:     []string{"mutation run", "mutation summarize"},
		},
		{
			name:     "no known command named",
			behavior: []string{"states plainly that it has no real work to do"},
			want:     nil,
		},
		{
			name:     "repeated command collapses",
			behavior: []string{"runs mutation run", "runs mutation run again after a fix"},
			want:     []string{"mutation run"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExpectedCommandOrder(c.behavior)
			if !equalStrings(got, c.want) {
				t.Errorf("ExpectedCommandOrder(%v) = %v, want %v", c.behavior, got, c.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeEventsLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadCommandOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	writeEventsLines(t, path,
		`{"at":"2026-01-01T00:00:00Z","type":"tool_run","command":"status"}`,
		`{"at":"2026-01-01T00:00:01Z","type":"stage_transition","stage":"plan","from":"pending","to":"running","command":"stage start"}`,
		`{"at":"2026-01-01T00:00:02Z","type":"guard_decision","command":"git push","allow":false}`,
		`{"at":"2026-01-01T00:00:03Z","type":"stage_transition","stage":"plan","from":"running","to":"awaiting_approval","command":"stage complete"}`,
	)

	got, err := ReadCommandOrder(path)
	if err != nil {
		t.Fatalf("ReadCommandOrder: %v", err)
	}
	want := []string{"status", "stage start", "stage complete"}
	if !equalStrings(got, want) {
		t.Errorf("ReadCommandOrder = %v, want %v (guard_decision must not be read as a call)", got, want)
	}
}

func TestReadCommandOrder_MissingFile(t *testing.T) {
	got, err := ReadCommandOrder(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatalf("ReadCommandOrder: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty for a missing events.jsonl", got)
	}
}

func TestIsSubsequence(t *testing.T) {
	cases := []struct {
		name string
		want []string
		got  []string
		ok   bool
	}{
		{"empty want always passes", nil, []string{"a", "b"}, true},
		{"exact match", []string{"a", "b"}, []string{"a", "b"}, true},
		{"other calls interleave", []string{"a", "b"}, []string{"a", "x", "b"}, true},
		{"wrong order", []string{"a", "b"}, []string{"b", "a"}, false},
		{"missing a call", []string{"a", "b"}, []string{"a"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSubsequence(c.want, c.got); got != c.ok {
				t.Errorf("IsSubsequence(%v, %v) = %v, want %v", c.want, c.got, got, c.ok)
			}
		})
	}
}
