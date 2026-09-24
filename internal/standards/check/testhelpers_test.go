package check_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// writeFile writes content to path, creating parent directories as
// needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// packWithConfig builds a *standards.Pack whose FS holds exactly one
// file at configPath — enough for materializeConfig/readConfigJSON to
// read a check's own Config through Check.Pack.FS, the same way a real
// synced or embedded pack would.
func packWithConfig(configPath, content string) *standards.Pack {
	return &standards.Pack{
		ID: "test", Layer: standards.LayerOrg, Ref: "test",
		FS: fstest.MapFS{configPath: &fstest.MapFile{Data: []byte(content)}},
	}
}

// packWithConfigDir builds a *standards.Pack whose FS holds every file
// in files (relative to configDir) — enough for materializeConfigDir
// (the "policy"/"scan" kinds' own directory-of-rego/policy-YAML
// config, phase 25) to copy a whole directory out of Check.Pack.FS.
func packWithConfigDir(configDir string, files map[string]string) *standards.Pack {
	mapFS := fstest.MapFS{}
	for name, content := range files {
		mapFS[configDir+"/"+name] = &fstest.MapFile{Data: []byte(content)}
	}
	return &standards.Pack{
		ID: "test", Layer: standards.LayerOrg, Ref: "test",
		FS: mapFS,
	}
}
