package evals

import (
	"regexp"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// queryToken is handoff.KeyPattern with its anchors dropped, so it can
// find a ticket key inside a longer string — a scenario's query is
// free text ("run mutation for PROJ-1"), not a single argument, so the
// key has to be picked out of it rather than matched whole. Derived
// from handoff.KeyPattern's own source (not a re-typed copy of the
// character class) so the two can never drift apart. Any text this
// finds is, by construction, exactly what handoff.KeyPattern would
// also accept.
var queryToken = regexp.MustCompile(strings.Trim(handoff.KeyPattern.String(), "^$"))

// ExtractKey returns the first ticket key named in query. A
// scenario's own query is the only place evals.json ever names the
// key a fixture is staged under — "develop PROJ-1" and "run mutation
// for PROJ-1" both resolve to "PROJ-1" — so this is how RunScenario
// decides which directory a staged fixture becomes.
func ExtractKey(query string) (string, bool) {
	m := queryToken.FindString(query)
	return m, m != ""
}
