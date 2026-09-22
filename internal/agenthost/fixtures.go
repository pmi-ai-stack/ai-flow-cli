package agenthost

import (
	"embed"
	"fmt"
	"sort"
)

// FixturesFS embeds every recorded hook payload under
// testdata/hooks/<host>/ — the payloads Guardrails names ("recorded
// payloads under testdata/hooks/<host>/") and what hook selftest
// drives at runtime, from the installed binary, not just from `go
// test`. See the plan's design decision 5: neither host's real
// payload shape was available to record from in this environment, so
// these are hand-built, documented as provisional.
//
//go:embed testdata/hooks
var FixturesFS embed.FS

// FixtureNames returns every fixture file name under
// testdata/hooks/<host>/ (e.g. "bash.json"), sorted, excluding
// "malformed.json" — the one fixture per host that deliberately is
// not valid JSON, exercised directly by each adapter's own parse-error
// test rather than by hook selftest's parse-and-compare loop.
func FixtureNames(host Host) ([]string, error) {
	entries, err := FixturesFS.ReadDir(fmt.Sprintf("testdata/hooks/%s", host))
	if err != nil {
		return nil, fmt.Errorf("agenthost: read testdata/hooks/%s: %w", host, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == "malformed.json" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// Fixture reads one fixture file's raw bytes.
func Fixture(host Host, name string) ([]byte, error) {
	data, err := FixturesFS.ReadFile(fmt.Sprintf("testdata/hooks/%s/%s", host, name))
	if err != nil {
		return nil, fmt.Errorf("agenthost: read testdata/hooks/%s/%s: %w", host, name, err)
	}
	return data, nil
}
