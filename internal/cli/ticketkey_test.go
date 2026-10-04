package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// TestEveryTicketScopedCommandRejectsMalformedKey enforces "every
// ticket-scoped command takes the ticket key as its first argument" by
// construction: any catalog command marked TicketScoped must refuse a
// malformed key with exit 2 and the key-validation message — before
// touching the home directory — so a new command cannot forget
// ValidateKey. The message check is what stops an unrelated usage error
// (a missing argument, a required flag) from passing this test.
func TestEveryTicketScopedCommandRejectsMalformedKey(t *testing.T) {
	const bad = "not-a-key"
	// keyOrDir lists commands whose first argument is a ticket key or a
	// directory (the dispatch is ticketKeyIfExists), so a string that is
	// not a key is a directory by design, not a missed ValidateKey. Its
	// own behaviour is covered by the TestStandardsCheckDirForm tests.
	keyOrDir := map[string]bool{"standards check": true}
	n := 0
	for _, spec := range handoff.Catalog() {
		if !spec.TicketScoped || keyOrDir[spec.Name] {
			continue
		}
		n++
		t.Run(spec.Name, func(t *testing.T) {
			home := t.TempDir()
			args := append(strings.Fields(spec.Name), bad)
			for range spec.Positional {
				args = append(args, "x")
			}
			args = append(args, "--home", home)

			var stdout, stderr strings.Builder
			ec := -1
			root := NewRoot(fixedBuildInfo, strings.NewReader(""), &stdout, &stderr, &ec)
			root.SetArgs(args)
			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("%v: cobra rejected the invocation before the command ran (%v); the test needs this command's required flags", args, err)
			}
			if ec != 2 {
				t.Fatalf("%v: exitCode = %d, want 2\n%s", args, ec, stdout.String())
			}
			var env struct {
				Error *struct{ Code, Message string } `json:"error"`
			}
			if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
				t.Fatalf("%v: stdout is not an envelope: %v\n%s", args, err, stdout.String())
			}
			if env.Error == nil || env.Error.Code != "usage" || !strings.Contains(env.Error.Message, "is not a valid ticket key") {
				t.Fatalf("%v: error = %+v, want usage \"is not a valid ticket key\"", args, env.Error)
			}
		})
	}
	if n == 0 {
		t.Fatal("handoff.Catalog() has no ticket-scoped commands; the test is not testing anything")
	}
}
