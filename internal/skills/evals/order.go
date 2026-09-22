package evals

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// commandNames is every Catalog() command name, longest first, so a
// substring search prefers a specific multi-word match ("mutation
// run") over any shorter name that might otherwise match inside it —
// memoized with sync.OnceValue, the same pattern
// internal/skills/lint/commands.go's own catalogByName already uses
// for "a lookup table computed once from handoff.Catalog()".
var commandNames = sync.OnceValue(func() []string {
	cat := handoff.Catalog()
	names := make([]string, len(cat))
	for i, c := range cat {
		names[i] = c.Name
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	return names
})

// ExpectedCommandOrder derives the ordered sequence of CLI calls an
// evals.json scenario's own expected_behavior[] implies, per the plan's
// design decision 3: each entry is scanned for the longest Catalog()
// command name it contains as a literal substring; an entry naming no
// known command contributes nothing (it is graded by Grade, not
// order-checked). Duplicated adjacent commands collapse to one — the
// order assertion cares about sequence, not repetition count.
func ExpectedCommandOrder(behavior []string) []string {
	var want []string
	for _, entry := range behavior {
		for _, name := range commandNames() {
			if strings.Contains(entry, name) {
				if len(want) == 0 || want[len(want)-1] != name {
					want = append(want, name)
				}
				break
			}
		}
	}
	return want
}

// eventLine is the subset of handoff.Event this package reads back —
// only Type and Command matter for call-order checking.
type eventLine struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// ReadCommandOrder reads eventsPath's "stage_transition" and
// "tool_run" events, in file order, returning their Command field —
// together the complete record of "the exact CLI calls were made"
// (design decision 1: every ticket-scoped command's Catalog() name
// ends up in one of these two event types). A missing events.jsonl
// (no command has run yet) returns an empty, non-nil-error slice — not
// every scenario necessarily drives a command before its transcript is
// graded.
func ReadCommandOrder(eventsPath string) ([]string, error) {
	data, err := handoff.ReadArtifact(eventsPath)
	if err != nil {
		return nil, err
	}
	var got []string
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var e eventLine
		if err := json.Unmarshal(line, &e); err != nil {
			continue // a malformed line never fails the assertion; it is simply not evidence
		}
		if (e.Type == handoff.EventTypeStageTransition || e.Type == handoff.EventTypeToolRun) && e.Command != "" {
			got = append(got, e.Command)
		}
	}
	return got, nil
}

// IsSubsequence reports whether want appears, in order, within got —
// not necessarily contiguously, since other real calls (stage start
// before a judgement command, for instance) legitimately interleave.
// An empty want is trivially satisfied: a scenario whose
// expected_behavior names no known command has nothing for this
// assertion to check, and that is a pass-through, not a failure.
func IsSubsequence(want, got []string) bool {
	i := 0
	for _, g := range got {
		if i == len(want) {
			break
		}
		if g == want[i] {
			i++
		}
	}
	return i == len(want)
}
