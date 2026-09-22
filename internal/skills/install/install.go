// Package install copies skills from a source directory (typically the
// ai-flow plugin's own skills/ directory) into a host's skills
// directory, for the no-plugin install path the Hosts table names:
// `ai-flow skills install --into .claude/skills` serves both hosts,
// `--into .agents/skills` is the Cursor-only form.
package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// Result is Install's return value.
type Result struct {
	// From is the source directory Install was called with.
	From string
	// Into is the destination directory Install was called with.
	Into string
	// Skills is every skill name copied, sorted (skills.DiscoverSkills's
	// own order).
	Skills []string
}

// Install copies every skill from/skills/<name>/ into into/<name>/,
// replacing any existing directory of that name so a re-run picks up a
// changed skill — see the plan's design decision 7 for why a plain
// os.CopyFS (which refuses to overwrite) is not enough on its own. Only
// the exact <name> subdirectories being installed are touched; any
// other entry already under into (a developer's own skill, or another
// plugin's) is left alone.
func Install(from, into string) (Result, error) {
	discovered, err := skills.DiscoverSkills(from)
	if err != nil {
		return Result{}, err
	}

	res := Result{From: from, Into: into}
	for _, s := range discovered {
		dst := filepath.Join(into, s.Name)
		if err := os.RemoveAll(dst); err != nil {
			return Result{}, fmt.Errorf("install: remove existing %s: %w", dst, err)
		}
		if err := os.CopyFS(dst, os.DirFS(s.Dir)); err != nil {
			return Result{}, fmt.Errorf("install: copy %s to %s: %w", s.Dir, dst, err)
		}
		res.Skills = append(res.Skills, s.Name)
	}
	return res, nil
}
