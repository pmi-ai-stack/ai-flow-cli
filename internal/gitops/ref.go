package gitops

import "github.com/pmi-ai-stack/ai-flow-cli/internal/gitref"

// endOfOptions ends git's option parsing before the first free
// argument: a configured base branch (or any ref ai-flow did not
// generate) that starts with "-" is then a ref name, never an option —
// `fetch origin '--upload-pack=<cmd>'` ran <cmd> before this existed.
// Every git verb this package calls honours it except checkout, which
// (git 2.34, verified) reads it as a pathspec; checkout's refs are
// therefore always validated with ValidRef first and followed by "--".
const endOfOptions = "--end-of-options"

// ValidRef is gitref.Valid — every ref this package puts on a git argv
// is checked with it first.
func ValidRef(name string) error { return gitref.Valid(name) }
