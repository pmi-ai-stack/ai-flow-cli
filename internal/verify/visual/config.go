package visual

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DevServerConfig is <repo>/.ai-flow/visual.json's dev_server block —
// the plan's design decision 5.
type DevServerConfig struct {
	Command               []string `json:"command"`
	URL                   string   `json:"url"`
	StartupTimeoutSeconds int      `json:"startup_timeout_seconds"`
}

// Config is <repo>/.ai-flow/visual.json's full shape.
type Config struct {
	DevServer      DevServerConfig `json:"dev_server"`
	CaptureCommand []string        `json:"capture_command"`
}

// defaultStartupTimeoutSeconds applies when a repository's visual.json
// omits startup_timeout_seconds (or sets it to zero) — no config
// surface names a value for this (the plan's design decision 5), so a
// fixed, generous default stands in.
const defaultStartupTimeoutSeconds = 30

// LoadConfig reads <repoLocalPath>/.ai-flow/visual.json. (nil, nil) —
// not an error — when the file does not exist: a repository that
// carries no visual contract at all is normal, and `visual run`'s own
// tool_missing error (naming this file) is the caller's job, not this
// function's.
func LoadConfig(repoLocalPath string) (*Config, error) {
	path := filepath.Join(repoLocalPath, ".ai-flow", "visual.json")
	data, err := os.ReadFile(path) //nolint:gosec // G304: repoLocalPath is repo.json's own local_path, explicit CLI input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("visual: read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("visual: parse %s: %w", path, err)
	}
	if cfg.DevServer.StartupTimeoutSeconds <= 0 {
		cfg.DevServer.StartupTimeoutSeconds = defaultStartupTimeoutSeconds
	}
	return &cfg, nil
}
