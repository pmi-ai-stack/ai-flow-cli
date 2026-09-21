package config

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

//go:embed template.json
var templateFS embed.FS

// templateBytes is the CLI's one copy of the config template — nothing
// in the plugin repository ships a second one. mustLoadTemplate parses
// it once at package load (a broken embedded template is a programmer
// error, per 04-errors.md rule 9's must* pattern) so Init never writes
// content this package has not already proven decodes cleanly; the
// full documented validation rules are proven separately by
// TestTemplatePassesValidate.
var templateBytes = mustLoadTemplate()

func mustLoadTemplate() []byte {
	data, err := templateFS.ReadFile("template.json")
	if err != nil {
		panic(fmt.Sprintf("config: embedded template.json missing: %v", err))
	}
	if _, err := decode(data); err != nil {
		panic(fmt.Sprintf("config: embedded template.json does not decode: %v", err))
	}
	return data
}

// Init writes the embedded template to ResolvePath(homeDir, configFlag)
// and returns that path. It refuses when the target already exists —
// the same "refuse, do not overwrite" shape as ops policy scaffold —
// rather than silently discarding a developer's hand-edited config.
func Init(homeDir, configFlag string) (string, error) {
	path := ResolvePath(homeDir, configFlag)

	switch _, err := os.Stat(path); {
	case err == nil:
		return "", &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("config.json already exists at %s", path),
			Details: []string{"edit it directly, or pass --config to target a different path"},
		}
	case errors.Is(err, os.ErrNotExist):
		// expected — this is the write path
	default:
		return "", fmt.Errorf("config: stat %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("config: create %s: %w", dir, err)
	}

	if err := atomicWrite(dir, path, templateBytes); err != nil {
		return "", err
	}
	return path, nil
}

// atomicWrite is 07-determinism.md's write shape: a temp file in the
// same directory, synced and closed, then renamed into place — so a
// crash mid-write never leaves a partial config.json.
func atomicWrite(dir, path string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".config.json.tmp-*")
	if err != nil {
		return fmt.Errorf("config: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("config: rename into place: %w", err)
	}
	cleanup = false
	return nil
}
