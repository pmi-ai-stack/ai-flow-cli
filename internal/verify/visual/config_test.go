package visual_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/visual"
)

func TestLoadConfigAbsent(t *testing.T) {
	cfg, err := visual.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg != nil {
		t.Errorf("LoadConfig(absent) = %+v, want nil", cfg)
	}
}

func TestLoadConfigPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ai-flow"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := `{
  "dev_server": { "command": ["npm", "run", "dev"], "url": "http://localhost:3000" },
  "capture_command": ["node", "scripts/capture.js"]
}`
	if err := os.WriteFile(filepath.Join(dir, ".ai-flow", "visual.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := visual.LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig(present) = nil, want a config")
	}
	if len(cfg.DevServer.Command) != 3 || cfg.DevServer.Command[0] != "npm" {
		t.Errorf("DevServer.Command = %v", cfg.DevServer.Command)
	}
	if cfg.DevServer.URL != "http://localhost:3000" {
		t.Errorf("DevServer.URL = %q", cfg.DevServer.URL)
	}
	if cfg.DevServer.StartupTimeoutSeconds != 30 {
		t.Errorf("StartupTimeoutSeconds = %d, want the default 30", cfg.DevServer.StartupTimeoutSeconds)
	}
	if len(cfg.CaptureCommand) != 2 {
		t.Errorf("CaptureCommand = %v", cfg.CaptureCommand)
	}
}

func TestLoadConfigMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ai-flow"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ai-flow", "visual.json"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := visual.LoadConfig(dir); err == nil {
		t.Error("LoadConfig(malformed): want an error")
	}
}
