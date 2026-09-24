package check

import "fmt"

// Target is the subset of an OPS target a "policy" or "scan" runner
// needs — mirroring Repo's own doc comment ("the subset of repo.json a
// runner needs"), not internal/ops.Target itself: this package stays
// free of an internal/ops import, the same decoupling Repo already
// gives internal/handoff's own repo.json. The caller (internal/cli's
// addon-scoped dispatch, phase 25) builds one per (check, OPS target)
// pair.
type Target struct {
	// Key is the target's canonical "<repo>:<path>" identity
	// (internal/ops.TargetKey) — carried through verbatim onto every
	// Finding this runner produces.
	Key string
	// Addon is the OPS addon that found this target ("terraform",
	// "kubectl", ...).
	Addon string
	// Repo is the target's own repository name.
	Repo string
	// SourceDir is the target's own directory inside the current
	// branch checkout (repo.LocalPath/target.Path) — what Check.Input
	// == "source" (the default) scans.
	SourceDir string
	// RenderDir is the target's already-written redacted rendering
	// directory (render-drift's own output — see
	// internal/ops/render.RedactedDir), empty when none has been
	// produced for this run. What Check.Input == "render" scans.
	RenderDir string
}

// targetInputDir resolves what a policy/scan runner should point its
// tool at, per in.Check.Input ("source", the default, or "render") —
// shared by runPolicy and runScan so the two kinds can never disagree
// about what "input: render" means. An empty result Target (Key=="")
// or an empty resolved directory is this package's caller's own bug
// (the addon-scoped dispatch loop in internal/cli/checks.go is
// responsible for only ever calling a policy/scan runner with a real
// Target, and for skipping a target whose rendering does not exist yet
// rather than calling in with an empty RenderDir) — reported as a
// plain error, not an *envelope.Error, since there is nothing a
// developer can act on by seeing it: 04-errors.md rule 9's must*
// shape without the panic.
func targetInputDir(in RunInput) (string, error) {
	if in.Target.Key == "" {
		return "", fmt.Errorf("check: %s: kind %q needs an OPS target, none provided", in.Check.ID, in.Check.Kind)
	}
	switch in.Check.Input {
	case "render":
		if in.Target.RenderDir == "" {
			return "", fmt.Errorf("check: %s: input \"render\" needs a rendering for target %q, none provided", in.Check.ID, in.Target.Key)
		}
		return in.Target.RenderDir, nil
	default: // "" or "source"
		if in.Target.SourceDir == "" {
			return "", fmt.Errorf("check: %s: no source directory for target %q", in.Check.ID, in.Target.Key)
		}
		return in.Target.SourceDir, nil
	}
}
