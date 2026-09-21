// Package cli builds the ai-flow root command: cobra commands, one file
// per noun, plus Catalog() and StageTable() for skills lint and status
// once phase 3 lands. As of phase 2 this carries the root, global flags,
// "version", "schema <artifact>" and "config init"/"config validate".
package cli

import "context"

// Flags are the global flags, parsed once by the root command's
// PersistentPreRunE and plumbed to every subcommand through a single
// context value — never through package-level state.
type Flags struct {
	// Home is --home, the ai-flow home directory. Every "~/.ai-flow/…"
	// path in the spec means this value.
	Home string
	// Config is --config, an explicit path to config.json overriding
	// the default <Home>/config.json.
	Config string
	// Simulate is --simulate: print the tracker, git and PR side
	// effects a command would perform, then exit 0 without performing
	// them. Every side-effecting command must consult it; its first
	// real consumers land in phases 4 and 5.
	Simulate bool
	// Verbose is --verbose: tool logs to stderr, secrets redacted.
	Verbose bool
	// Human is --human. --json is the default (Human == false).
	Human bool
}

type flagsCtxKey struct{}

// WithFlags returns a context carrying f, retrievable with FlagsFrom.
func WithFlags(ctx context.Context, f Flags) context.Context {
	return context.WithValue(ctx, flagsCtxKey{}, f)
}

// FlagsFrom returns the Flags carried by ctx, or the zero value if none
// were set — which happens only if a command is invoked outside the
// root's PersistentPreRunE, which never occurs in production use.
func FlagsFrom(ctx context.Context) Flags {
	f, _ := ctx.Value(flagsCtxKey{}).(Flags)
	return f
}
