package evals

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills/install"
)

// Workspace is one scenario's isolated run: a synthetic $HOME a
// headless host's subprocess (and every `ai-flow` call it shells out
// to as a grandchild) is launched with, so `~/.ai-flow` resolves to a
// temporary directory without a skill's own body ever passing --home
// itself — see the phase 11 plan's design decision 2 — plus a project
// directory the host's cwd points at, carrying the staged skill(s)
// under .claude/skills/.
type Workspace struct {
	// HomeDir is set as the subprocess's HOME environment variable.
	HomeDir string
	// ProjectDir is the headless host's working directory.
	ProjectDir string
	// Key is the ticket key the staged fixture is written under.
	Key string
}

// NewWorkspace builds a fresh Workspace under root/<key>/ (root is
// typically a scenario-scoped temporary directory the caller already
// created) and writes the CLI's embedded config.json template into
// AIFlowHome(), so every `ai-flow` call the headless host makes has a
// config to load. It does not stage a fixture or a skill — see
// StageFixture and StageSkills.
func NewWorkspace(root, key string) (*Workspace, error) {
	w := &Workspace{
		HomeDir:    filepath.Join(root, "home"),
		ProjectDir: filepath.Join(root, "project"),
		Key:        key,
	}

	if err := os.MkdirAll(w.AIFlowHome(), 0o700); err != nil {
		return nil, fmt.Errorf("evals: create %s: %w", w.AIFlowHome(), err)
	}
	if err := os.MkdirAll(w.ProjectDir, 0o700); err != nil {
		return nil, fmt.Errorf("evals: create %s: %w", w.ProjectDir, err)
	}
	if _, err := config.Init(w.AIFlowHome(), ""); err != nil {
		return nil, fmt.Errorf("evals: write config.json: %w", err)
	}
	return w, nil
}

// AIFlowHome returns HomeDir/.ai-flow — exactly what
// internal/cli.defaultHome() would resolve --home to from HomeDir.
func (w *Workspace) AIFlowHome() string {
	return filepath.Join(w.HomeDir, ".ai-flow")
}

// TicketDir returns this workspace's staged ticket directory,
// AIFlowHome()/Key.
func (w *Workspace) TicketDir() string {
	return filepath.Join(w.AIFlowHome(), w.Key)
}

// EventsPath returns this workspace's staged ticket's events.jsonl
// path.
func (w *Workspace) EventsPath() string {
	return filepath.Join(w.TicketDir(), "events.jsonl")
}

// StageFixture copies fixtureDir's contents into w.TicketDir() — a
// recorded handoff directory (status.json, and whatever else the
// scenario's own artifacts include) becomes the staged ticket a
// headless host's `ai-flow` calls operate on. It refuses when
// w.TicketDir() already exists — AI_FLOW_FIXTURE_DIR's own documented
// fallback, "refused when a real ticket directory exists" (see the
// plan's design decision 7); unreachable in this package's own normal
// use, since every Workspace is freshly built, but proven by a
// dedicated test.
func (w *Workspace) StageFixture(fixtureDir string) error {
	dst := w.TicketDir()
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("evals: refusing to stage fixture: %s already exists", dst)
	}
	if err := os.CopyFS(dst, os.DirFS(fixtureDir)); err != nil {
		return fmt.Errorf("evals: stage fixture %s into %s: %w", fixtureDir, dst, err)
	}
	return nil
}

// StageSkills installs every skill internal/skills.DiscoverSkills finds
// under pluginRoot into w.ProjectDir/.claude/skills/ — the project
// skills directory a Claude Code headless session searches from its
// own cwd (internal/agenthost/claudecode.Adapter.SkillsDirs). All
// skills under pluginRoot are installed, not just the one under test,
// since a scenario's own skill (develop, review-changes) may itself
// load another by name.
func (w *Workspace) StageSkills(pluginRoot string) error {
	into := filepath.Join(w.ProjectDir, ".claude", "skills")
	if _, err := install.Install(pluginRoot, into); err != nil {
		return err
	}
	return nil
}
