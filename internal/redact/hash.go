package redact

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashPrefixLen is the number of hex characters (4 bytes) of the digest
// kept in a redacted token — long enough to tell two different secrets
// apart in one log without printing anything reversible.
const hashPrefixLen = 8

// HashPrefix returns the token substituted for a redacted value:
// <redacted:xxxxxxxx>, xxxxxxxx being the first hashPrefixLen hex
// characters of sha256(value). One formula, shared by environment-
// variable redaction (Redactor.String/Bytes) and marking-driven
// redaction (RedactMarked) — both produce identical-shaped tokens for
// the same input.
func HashPrefix(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "<redacted:" + hex.EncodeToString(sum[:])[:hashPrefixLen] + ">"
}
