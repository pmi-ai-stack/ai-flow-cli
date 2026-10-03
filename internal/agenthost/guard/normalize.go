package guard

import (
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// resolve is the one spelling every path the guard compares is brought
// to: absolute (a relative p is joined to base, a relative base to the
// process's own working directory), cleaned, and with symlinks
// resolved. Without it a relative file_path, a symlink into the ticket
// directory, or a relative --home each made every path row silently
// allow.
//
// Symlinks are resolved on the deepest ancestor that exists and the
// rest is re-joined: filepath.EvalSymlinks fails on a path that does
// not exist yet, and a new file is the common write — falling back to
// the unresolved path there would fail open on exactly the writes the
// guard exists for. An empty p stays empty.
func resolve(p, base string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}

	existing, rest := abs, ""
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs // nothing on this path exists, not even the root
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return abs
	}
	return filepath.Join(real, rest)
}

// normalized is in with every path field resolved: home against the
// process cwd, the event's file path and cwd against the event's own
// cwd, and every path active.json lists. Active is copied, never
// mutated — the caller's value is the file's contents.
func normalized(in Input) (agenthost.HookEvent, *handoff.Active, string) {
	home := resolve(in.Home, "")
	ev := in.Event
	cwd := resolve(ev.Cwd, "")
	ev.FilePath = resolve(ev.FilePath, cwd)
	ev.Cwd = cwd

	if in.Active == nil {
		return ev, nil, home
	}
	a := *in.Active
	a.RepoRoots = resolveAll(a.RepoRoots)
	a.ToolConfigFiles = resolveAll(a.ToolConfigFiles)
	return ev, &a, home
}

func resolveAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = resolve(p, "")
	}
	return out
}
