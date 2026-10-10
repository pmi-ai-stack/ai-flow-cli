package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestConfigInitTrackerGitHubValidates: `config init --tracker github`
// writes a GitHub Issues config that `config validate` accepts as is.
func TestConfigInitTrackerGitHubValidates(t *testing.T) {
	home := t.TempDir()
	if stdout, _, ec := runRoot(t, "config", "init", "--tracker", "github", "--home", home); ec != 0 {
		t.Fatalf("config init --tracker github: exitCode = %d\n%s", ec, stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Tracker struct {
			Provider     string            `json:"provider"`
			BaseURL      string            `json:"base_url"`
			Repositories map[string]string `json:"repositories"`
		} `json:"tracker"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Tracker.Provider != "github" || cfg.Tracker.BaseURL != "https://api.github.com" || len(cfg.Tracker.Repositories) == 0 {
		t.Errorf("tracker = %+v, want provider github, base_url https://api.github.com, a repositories entry", cfg.Tracker)
	}
	if stdout, _, ec := runRoot(t, "config", "validate", "--home", home); ec != 0 {
		t.Fatalf("config validate: exitCode = %d\n%s", ec, stdout.String())
	}
}

// TestConfigInitTrackerDefaultIsJira: no flag and --tracker jira write
// the same bytes.
func TestConfigInitTrackerDefaultIsJira(t *testing.T) {
	read := func(args ...string) string {
		t.Helper()
		home := t.TempDir()
		if stdout, _, ec := runRoot(t, append(append([]string{"config", "init"}, args...), "--home", home)...); ec != 0 {
			t.Fatalf("config init %v: exitCode = %d\n%s", args, ec, stdout.String())
		}
		data, err := os.ReadFile(filepath.Join(home, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if read() != read("--tracker", "jira") {
		t.Error("config init and config init --tracker jira wrote different files")
	}
}

// TestConfigInitTrackerUnknownIsUsage: an unsupported tracker is exit 2
// usage and writes nothing.
func TestConfigInitTrackerUnknownIsUsage(t *testing.T) {
	home := t.TempDir()
	stdout, _, ec := runRoot(t, "config", "init", "--tracker", "gitlab", "--home", home)
	if ec != 2 {
		t.Fatalf("exitCode = %d, want 2\n%s", ec, stdout.String())
	}
	var env struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.Error == nil || env.Error.Code != "usage" {
		t.Errorf("error = %+v, want code=usage", env.Error)
	}
	if _, err := os.Stat(filepath.Join(home, "config.json")); !os.IsNotExist(err) {
		t.Errorf("config.json exists after a refused init (stat err = %v)", err)
	}
}
