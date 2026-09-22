package handoff

import (
	"encoding/json"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// Active is ~/.ai-flow/active.json: the one system-wide record of
// whichever ticket is currently in progress, rewritten by every
// mutating stage command (start/complete/approve/skip/reset). There is
// no per-ticket active.json — see the plan's design decision 7.
type Active struct {
	Key             string   `json:"key"`
	Stage           string   `json:"stage"`
	RepoRoots       []string `json:"repo_roots"`
	ToolConfigFiles []string `json:"tool_config_files"`
	AllowlistHash   string   `json:"allowlist_hash"`
	LastEventAt     string   `json:"last_event_at"`
}

// LoadActive reads home's active.json, returning (nil, nil) when it
// does not exist — no active ticket is a normal state (doctor reports
// on it in phase 8; the guard reads it starting phase 9).
func LoadActive(home string) (*Active, error) {
	data, err := ReadArtifact(ActivePath(home))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var a Active
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// SaveActive writes a to home's active.json.
func SaveActive(home string, a *Active) (string, error) {
	return WriteRaw(home, "active.json", a)
}

// repoJSONRepositories is the subset of repo.json's shape RecordActive
// needs — a tolerant decode, mirroring predicates.go's checksResults.
type repoJSONRepositories struct {
	Repositories []struct {
		LocalPath string `json:"local_path"`
	} `json:"repositories"`
}

// repoRootsFor reads home's repo.json for key (if it exists yet — it
// does not before `locate` completes) and returns every repository's
// local_path, sorted and deduped. ToolConfigFiles stays empty until
// phase 14 populates it.
func repoRootsFor(home, key string) ([]string, error) {
	data, err := ReadArtifact(RepoPath(home, key))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return []string{}, nil
	}
	var r repoJSONRepositories
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	var paths []string
	for _, repo := range r.Repositories {
		paths = append(paths, repo.LocalPath)
	}
	roots := strutil.DedupSorted(paths)
	if roots == nil {
		roots = []string{}
	}
	return roots, nil
}

// RecordActive rebuilds and saves active.json for key/stage — the call
// every mutating stage command makes after a successful transition.
// RepoRoots is read back from repo.json when one exists yet (empty
// before `locate` completes); ToolConfigFiles stays empty until
// phase 14 populates it; AllowlistHash reflects exec.Table as it
// stands today.
func RecordActive(home, key string, stage Stage) error {
	roots, err := repoRootsFor(home, key)
	if err != nil {
		return err
	}
	a := &Active{
		Key:             key,
		Stage:           string(stage),
		RepoRoots:       roots,
		ToolConfigFiles: []string{},
		AllowlistHash:   exec.Hash(),
		LastEventAt:     Now().UTC().Format(time.RFC3339),
	}
	_, err = SaveActive(home, a)
	return err
}
