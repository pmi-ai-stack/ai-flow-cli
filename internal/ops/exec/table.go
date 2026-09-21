package exec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Table is the allowlist this binary actually ships. It is empty in
// phase 1 through phase 3 — every later phase appends its own rows
// here, in the same PR that needs them (see
// TestAllowlistNeverMutatesPersistentState in allowlist_test.go, which
// runs against exactly this table). internal/handoff hashes it into
// active.json's allowlist_hash from phase 3 on, so the guard (phase 9)
// and doctor (phase 8) can tell an active.json written by a different
// CLI version apart from one written by this build, long before the
// table itself has any real rows.
var Table []Row

// Hash returns a stable hex-encoded SHA-256 digest of Table's current
// contents. It changes exactly when a row is added, removed or
// modified — never on call order, since Table is a plain slice marshalled
// as a JSON array (order is already the table's own declared order, not
// incidental map iteration).
func Hash() string {
	data, err := json.Marshal(Table)
	if err != nil {
		// Row is a plain struct of strings, slices of strings and bools —
		// it cannot fail to marshal. A failure here is a programmer error
		// (04-errors.md rule 9's must* pattern), not an input this
		// function's caller can act on.
		panic(fmt.Sprintf("exec: marshal Table: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
