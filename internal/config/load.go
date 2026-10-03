package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// DefaultGuardIdleHours is applied when config.defaults.guard_idle_hours
// is absent or zero (see Config.GuardIdleHours's doc comment). Exported
// so a caller that cannot load config.json at all (doctor, when it is
// missing or invalid) can still apply the same default doctor would see
// once it does load, instead of restating the literal.
const DefaultGuardIdleHours = 8

// DefaultHTTPTimeoutSeconds is applied when
// config.defaults.http_timeout_seconds is absent or non-positive, and
// by callers that run without a loaded config (doctor on a broken
// config.json) through Defaults.HTTPTimeout.
const DefaultHTTPTimeoutSeconds = 30

// ResolvePath returns the config.json path Load and Init use: configFlag
// verbatim when non-empty (the --config flag), else
// <homeDir>/config.json.
func ResolvePath(homeDir, configFlag string) string {
	if configFlag != "" {
		return configFlag
	}
	return filepath.Join(homeDir, "config.json")
}

// Load reads, decodes and backfills defaults into the config at
// ResolvePath(homeDir, configFlag). It does not run Validate — callers
// that need the documented validation rules call Validate separately,
// so Load stays usable by commands (like `stage start`) that only need
// a config to already be well-formed, not freshly re-validated on every
// invocation.
//
// A missing file is *envelope.Error{Code: "missing_artifact"} (exit 3);
// malformed JSON, an unknown field or an unsupported schema_version is
// *envelope.Error{Code: "config_invalid"} (exit 6).
func Load(homeDir, configFlag string) (*Config, error) {
	path := ResolvePath(homeDir, configFlag)

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is --config/--home, explicit CLI input by design, not attacker-controlled
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &envelope.Error{
				Code:    "missing_artifact",
				Message: fmt.Sprintf("config.json not found at %s; run `ai-flow config init` first", path),
			}
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	cfg, err := decode(data)
	if err != nil {
		return nil, &envelope.Error{
			Code:    "config_invalid",
			Message: fmt.Sprintf("%s: %s", path, err.Error()),
		}
	}

	if cfg.SchemaVersion != CurrentSchemaVersion {
		return nil, &envelope.Error{
			Code: "config_invalid",
			Message: fmt.Sprintf("%s: schema_version %d is not supported (this binary reads %d only; there are no config upgrades yet)",
				path, cfg.SchemaVersion, CurrentSchemaVersion),
		}
	}

	applyDefaults(cfg)
	return cfg, nil
}

// decode parses data into a Config, rejecting any field the schema
// does not name (07-determinism.md: a typo in config.json must fail
// config validate, not silently default) and any trailing content after
// the one top-level object.
func decode(data []byte) (*Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if dec.More() {
		return nil, errors.New("decode: trailing content after the top-level object")
	}
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Defaults.GuardIdleHours == 0 {
		cfg.Defaults.GuardIdleHours = DefaultGuardIdleHours
	}
	if cfg.Defaults.HTTPTimeoutSeconds <= 0 {
		cfg.Defaults.HTTPTimeoutSeconds = DefaultHTTPTimeoutSeconds
	}
}
