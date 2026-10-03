package cli

import (
	"os"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// envelopeRedactor is the Redactor wrap applies to every envelope it
// prints — an injectable constructor (git.go's newExecer pattern) so a
// test decides which environment it sees instead of inheriting the test
// process's own, and production reads os.Environ() at print time.
var envelopeRedactor = func() *redact.Redactor { return redact.New(os.Environ()) }
