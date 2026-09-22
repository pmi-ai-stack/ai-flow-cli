package detect

import "encoding/json"

// packageJSON is the subset of package.json this package reads — a
// tolerant decode, exactly like internal/handoff's own tolerant
// decodes of artifacts it only partially cares about.
type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// readPackageJSON reads dir/package.json, returning (nil, false, nil)
// when it does not exist.
func readPackageJSON(dir string) (*packageJSON, bool, error) {
	data, ok, err := readFile(dir, "package.json")
	if err != nil || !ok {
		return nil, ok, err
	}
	var pj packageJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		// A package.json that does not even parse as JSON carries no
		// usable signal — treated as absent, not a hard failure, so one
		// malformed repository does not fail detection across a whole
		// multi-repository ticket.
		return nil, false, nil //nolint:nilerr // deliberate: malformed package.json is "no signal", see comment above
	}
	return &pj, true, nil
}

// hasDependency reports whether pj names dep in either dependency map.
func (pj *packageJSON) hasDependency(dep string) bool {
	if pj == nil {
		return false
	}
	if _, ok := pj.Dependencies[dep]; ok {
		return true
	}
	_, ok := pj.DevDependencies[dep]
	return ok
}
