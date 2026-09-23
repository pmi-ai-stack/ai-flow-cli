package render

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// markingsFile is the real cache dir's own private sidecar, not an
// artifact and not part of any of the 17 handoff schemas: it lets a
// later cache hit — possibly for a different ticket than the one that
// first rendered this content — still know what to redact when
// building its own copy, without re-running the addon. It sits inside
// the 0700 cache directory alongside the real (already sensitive)
// rendered files, so it introduces no new place a secret is written
// that was not already true of the rendering itself.
//
// Written and read through internal/handoff's own WriteRaw/ReadArtifact
// (the same atomic-write and tolerant-missing-file primitives every
// other non-schema JSON file in this codebase uses, e.g. active.json)
// rather than a second, hand-rolled os.WriteFile/os.ReadFile pair.
const markingsFile = ".markings.json"

func writeMarkings(dir string, marked []redact.MarkedValue) error {
	if _, err := handoff.WriteRaw(dir, markingsFile, marked); err != nil {
		return fmt.Errorf("render: write markings: %w", err)
	}
	return nil
}

func readMarkings(dir string) ([]redact.MarkedValue, error) {
	data, err := handoff.ReadArtifact(filepath.Join(dir, markingsFile))
	if err != nil {
		return nil, fmt.Errorf("render: read markings: %w", err)
	}
	if data == nil {
		return nil, nil
	}
	var marked []redact.MarkedValue
	if err := json.Unmarshal(data, &marked); err != nil {
		return nil, fmt.Errorf("render: decode markings: %w", err)
	}
	return marked, nil
}
