package detect

// MutationMatch is the detected mutation tool for one repository.
// Built now (§ the plan's own text: "internal/detect: runners,
// mutation, e2e, frontend, languages"); not yet consumed by a skip
// rule or a runner — phase 7 wires both, per the plan's design
// decision 1. No Repo field — see RunnerMatch's own doc comment.
type MutationMatch struct {
	Tool string // "gremlins", "stryker", "mutmut", "pitest"
}

// Mutation detects repo's mutation testing tool. ok is false when
// nothing matches (e.g. a csharp-only repository — no mutation tool
// is named for csharp in the spec's phase-1 tool list).
func Mutation(repo Repo) (MutationMatch, bool, error) {
	dir := repo.LocalPath

	if hasFile(dir, "go.mod") {
		return MutationMatch{Tool: "gremlins"}, true, nil
	}

	pj, ok, err := readPackageJSON(dir)
	if err != nil {
		return MutationMatch{}, false, err
	}
	if ok && (pj.hasDependency("@stryker-mutator/core") || hasAnyGlob(dir, "stryker.conf.*")) {
		return MutationMatch{Tool: "stryker"}, true, nil
	}

	if hasFile(dir, "pyproject.toml") || hasFile(dir, "setup.py") {
		return MutationMatch{Tool: "mutmut"}, true, nil
	}

	if hasFile(dir, "pom.xml") {
		return MutationMatch{Tool: "pitest"}, true, nil
	}

	return MutationMatch{}, false, nil
}
