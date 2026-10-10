package config

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

//go:embed template.json template_tracker_github.json
var templateFS embed.FS

// templateBytes is the CLI's one copy of the config template — nothing
// in the plugin repository ships a second one. mustLoadTemplate parses
// it once at package load (a broken embedded template is a programmer
// error, per 04-errors.md rule 9's must* pattern) so Init never writes
// content this package has not already proven decodes cleanly; the
// full documented validation rules are proven separately by
// TestTemplatePassesValidate.
var templateBytes = mustLoadTemplate()

// githubTemplateBytes is `config init --tracker github`'s file:
// template.json with only its tracker value replaced by
// template_tracker_github.json, so the two templates cannot drift
// apart anywhere else. Proven to decode at package load, like
// templateBytes.
var githubTemplateBytes = mustSpliceTracker("template_tracker_github.json")

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

func mustSpliceTracker(fragment string) []byte {
	tracker, err := templateFS.ReadFile(fragment)
	if err != nil {
		panic(fmt.Sprintf("config: embedded %s missing: %v", fragment, err))
	}
	data, err := spliceValue(templateBytes, "tracker", tracker)
	if err != nil {
		panic(fmt.Sprintf("config: splice %s into template.json: %v", fragment, err))
	}
	if _, err := decode(data); err != nil {
		panic(fmt.Sprintf("config: template with %s does not decode: %v", fragment, err))
	}
	return data
}

// Template returns the config.json `config init --tracker provider`
// writes: template.json itself for jira, the spliced GitHub Issues
// variant for github.
func Template(provider string) ([]byte, error) {
	switch provider {
	case TrackerJira:
		return templateBytes, nil
	case TrackerGitHub:
		return githubTemplateBytes, nil
	default:
		return nil, &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("unknown tracker %q", provider),
			Details: []string{"--tracker accepts " + TrackerJira + " or " + TrackerGitHub},
		}
	}
}

// spliceValue replaces the value of the top-level key in the JSON
// object data with value, re-indented one level to sit where the old
// value did. Every byte outside that value is data's own.
func spliceValue(data []byte, key string, value []byte) ([]byte, error) {
	start, end, err := valueSpan(data, key)
	if err != nil {
		return nil, err
	}
	indented := strings.ReplaceAll(strings.TrimSpace(string(value)), "\n", "\n  ")
	out := make([]byte, 0, len(data)-(end-start)+len(indented))
	out = append(out, data[:start]...)
	out = append(out, indented...)
	return append(out, data[end:]...), nil
}

// valueSpan is the byte range [start, end) of the top-level key's
// value in the JSON object data.
func valueSpan(data []byte, key string) (int, int, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return 0, 0, errors.New("not a JSON object")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return 0, 0, err
		}
		afterKey := int(dec.InputOffset())
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return 0, 0, err
		}
		if tok != key {
			continue
		}
		end := int(dec.InputOffset())
		start := afterKey + bytes.Index(data[afterKey:end], raw[:1])
		return start, end, nil
	}
	return 0, 0, fmt.Errorf("no top-level %q key", key)
}

// FromTemplate is the configuration `config init` would write, already
// loaded — for the one command that runs without a config.json at all
// (`standards check <dir>` on a CI runner). Every call decodes a fresh
// copy, so a caller may modify what it gets.
func FromTemplate() *Config {
	cfg, err := decode(templateBytes)
	if err != nil {
		panic(fmt.Sprintf("config: embedded template.json does not decode: %v", err)) // proven at package load
	}
	applyDefaults(cfg)
	return cfg
}

// Init writes the embedded (jira) template to ResolvePath(homeDir,
// configFlag) and returns that path; InitFor names the tracker.
func Init(homeDir, configFlag string) (string, error) {
	return InitFor(homeDir, configFlag, TrackerJira)
}

// InitFor writes Template(provider) to ResolvePath(homeDir, configFlag)
// and returns that path. It refuses when the target already exists —
// the same "refuse, do not overwrite" shape as ops policy scaffold —
// rather than silently discarding a developer's hand-edited config.
func InitFor(homeDir, configFlag, provider string) (string, error) {
	data, err := Template(provider)
	if err != nil {
		return "", err
	}
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

	if err := atomicWrite(dir, path, data); err != nil {
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
