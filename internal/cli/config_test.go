package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runRoot(t *testing.T, args ...string) (stdout, stderr bytes.Buffer, exitCode int) {
	t.Helper()
	exitCode = -1
	root := NewRoot(fixedBuildInfo, &stdout, &stderr, &exitCode)
	root.SetArgs(args)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute(%v): %v", args, err)
	}
	return stdout, stderr, exitCode
}

func TestConfigInitJSON(t *testing.T) {
	home := t.TempDir()
	stdout, _, exitCode := runRoot(t, "config", "init", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\nstdout: %s", exitCode, stdout.String())
	}

	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Path string `json:"path"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if !env.OK {
		t.Fatal("ok = false, want true")
	}
	want := filepath.Join(home, "config.json")
	if env.Result.Path != want {
		t.Errorf("result.path = %q, want %q", env.Result.Path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("config.json was not written: %v", err)
	}
}

func TestConfigInitRefusesExistingViaCLI(t *testing.T) {
	home := t.TempDir()
	if _, _, exitCode := runRoot(t, "config", "init", "--home", home); exitCode != 0 {
		t.Fatalf("first init: exitCode = %d, want 0", exitCode)
	}

	stdout, _, exitCode := runRoot(t, "config", "init", "--home", home)
	if exitCode != 2 {
		t.Errorf("second init: exitCode = %d, want 2 (usage)", exitCode)
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
}

func TestConfigValidateOnFreshInit(t *testing.T) {
	home := t.TempDir()
	if _, _, exitCode := runRoot(t, "config", "init", "--home", home); exitCode != 0 {
		t.Fatalf("init: exitCode = %d, want 0", exitCode)
	}

	stdout, _, exitCode := runRoot(t, "config", "validate", "--home", home)
	if exitCode != 0 {
		t.Fatalf("validate: exitCode = %d, want 0\nstdout: %s", exitCode, stdout.String())
	}

	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Path          string `json:"path"`
			SchemaVersion int    `json:"schema_version"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if !env.OK {
		t.Fatal("ok = false, want true")
	}
	if env.Result.SchemaVersion != 1 {
		t.Errorf("result.schema_version = %d, want 1", env.Result.SchemaVersion)
	}
}

func TestConfigValidateMissingFile(t *testing.T) {
	home := t.TempDir()
	stdout, _, exitCode := runRoot(t, "config", "validate", "--home", home)
	if exitCode != 3 {
		t.Errorf("exitCode = %d, want 3 (precondition/missing_artifact)\nstdout: %s", exitCode, stdout.String())
	}
	var env struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.Error == nil || env.Error.Code != "missing_artifact" {
		t.Errorf("error = %+v, want code=missing_artifact", env.Error)
	}
}

func TestConfigValidateBrokenConfig(t *testing.T) {
	home := t.TempDir()
	broken := `{"schema_version": 1, "ops": {"risk_gate": 999, "default_environment": "nope", "environments": {}}}`
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(broken), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	stdout, _, exitCode := runRoot(t, "config", "validate", "--home", home)
	if exitCode != 6 {
		t.Errorf("exitCode = %d, want 6 (config_invalid)\nstdout: %s", exitCode, stdout.String())
	}
	var env struct {
		Error *struct {
			Code    string   `json:"code"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.Error == nil || env.Error.Code != "config_invalid" {
		t.Fatalf("error = %+v, want code=config_invalid", env.Error)
	}
	if len(env.Error.Details) < 2 {
		t.Errorf("details = %v, want at least 2 (risk_gate and default_environment both violated)", env.Error.Details)
	}
	joined := strings.Join(env.Error.Details, " | ")
	if !strings.Contains(joined, "risk_gate") || !strings.Contains(joined, "default_environment") {
		t.Errorf("details = %v, want both risk_gate and default_environment violations", env.Error.Details)
	}
}
