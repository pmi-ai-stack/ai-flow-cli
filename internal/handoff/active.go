package handoff

import (
	"encoding/json"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
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

// RecordActive rebuilds and saves active.json for key/stage — the call
// every mutating stage command makes after a successful transition.
// RepoRoots and ToolConfigFiles stay empty until phases 5 and 14
// populate them; AllowlistHash reflects exec.Table as it stands today
// (empty through phase 3, extended without any change here starting
// phase 5).
func RecordActive(home, key string, stage Stage) error {
	a := &Active{
		Key:             key,
		Stage:           string(stage),
		RepoRoots:       []string{},
		ToolConfigFiles: []string{},
		AllowlistHash:   exec.Hash(),
		LastEventAt:     Now().UTC().Format(time.RFC3339),
	}
	_, err := SaveActive(home, a)
	return err
}
