package agenthost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ScanForManifest is the plumbing every Adapter.Detect shares: walk
// candidates in order, looking for manifestRelPath (e.g.
// ".claude-plugin/plugin.json") under each; return the first match's
// "version" field and containing root. "Not found under any
// candidate" is (Detection{}, false, nil), matching Adapter.Detect's
// own contract — a real error is reserved for an actual filesystem
// problem, never for "not installed here".
func ScanForManifest(candidates []string, manifestRelPath string) (Detection, bool, error) {
	for _, root := range candidates {
		path := filepath.Join(root, manifestRelPath)
		data, err := os.ReadFile(path) //nolint:gosec // G304: path is built from a fixed candidate list, not attacker input
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Detection{}, false, fmt.Errorf("read %s: %w", path, err)
		}
		var m struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			return Detection{}, false, fmt.Errorf("decode %s: %w", path, err)
		}
		return Detection{Version: m.Version, PluginRoot: root}, true, nil
	}
	return Detection{}, false, nil
}
