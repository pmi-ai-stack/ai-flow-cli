package detect

// RunnerMatch is the detected unit test runner for one repository:
// enough for internal/verify/unit's Run to build the exact argv, and
// for internal/verify/unit to pick the matching parser by Runner. No
// Repo field — every caller already has the detect.Repo (and its
// Name) in hand from the call to Runners itself.
type RunnerMatch struct {
	Runner  string // "go", "jest", "pytest" — also the parser selector
	Command string
	Args    []string
}

// Runners detects repo's unit test runner. ok is false when nothing
// matches — internal/cli's `test run` skips such a repository rather
// than failing the whole command (see the plan's design decision 10).
func Runners(repo Repo) (RunnerMatch, bool, error) {
	dir := repo.LocalPath

	if hasFile(dir, "go.mod") {
		return RunnerMatch{Runner: "go", Command: "go", Args: []string{"test", "-json", "./..."}}, true, nil
	}

	pj, ok, err := readPackageJSON(dir)
	if err != nil {
		return RunnerMatch{}, false, err
	}
	if ok && (pj.hasDependency("jest") || hasAnyGlob(dir, "jest.config.*")) {
		return RunnerMatch{Runner: "jest", Command: "npx", Args: []string{"jest", "--ci", "--json"}}, true, nil
	}

	if hasFile(dir, "pyproject.toml") || hasFile(dir, "setup.cfg") || hasFile(dir, "pytest.ini") {
		return RunnerMatch{Runner: "pytest", Command: "pytest"}, true, nil
	}

	return RunnerMatch{}, false, nil
}
