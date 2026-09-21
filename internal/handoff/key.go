package handoff

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// KeyPattern is the ticket-key shape every other entry under
// ~/.ai-flow/ is defined not to collide with: a key always starts with
// an uppercase letter, so no reserved lowercase name can ever equal
// one (see TestReservedNamesNeverMatchKeyPattern).
var KeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+$`)

// ReservedNames is every non-ticket entry under ~/.ai-flow/. A future
// entry added at that level must be added here too, in the same PR.
var ReservedNames = []string{"config.json", "active.json", "events.jsonl", "standards", "cache"}

// ValidateKey reports whether key matches KeyPattern. A command
// receiving an invalid key returns *envelope.Error{Code: "usage"}
// (exit 2) — a malformed key is a caller bug, not a documented
// exit-4 ambiguity.
func ValidateKey(key string) error {
	if KeyPattern.MatchString(key) {
		return nil
	}
	return &envelope.Error{
		Code:    "usage",
		Message: fmt.Sprintf("%q is not a valid ticket key (expected pattern %s)", key, KeyPattern.String()),
	}
}

// IsReserved reports whether name is one of ReservedNames.
func IsReserved(name string) bool {
	return slices.Contains(ReservedNames, name)
}
