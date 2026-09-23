package check

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// dependencyConfig mirrors packs/reference/languages/<lang>/checks/dependencies.json.
// allowed_licenses and fail_on_advisory_severity are read but produce
// no finding — neither a license nor an advisory feed is available
// from a bare checkout without a network call, which this package (like
// every allowlisted tool) never makes on a check's behalf. A future
// phase that adds one extends runDependency, not this shape.
type dependencyConfig struct {
	AllowedLicenses        []string          `json:"allowed_licenses"`
	BannedPackages         []string          `json:"banned_packages"`
	MinimumVersions        map[string]string `json:"minimum_versions"`
	FailOnAdvisorySeverity string            `json:"fail_on_advisory_severity"`
}

// runDependency implements the "dependency" kind: a built-in check
// (no Tool, no subprocess) reading dependencies.json plus the
// repository's own manifest file. x/timeout are accepted only to
// satisfy CheckRunner's shape — this runner starts nothing.
func runDependency(_ context.Context, in RunInput, _ exec.Execer, _ time.Duration) ([]Finding, error) {
	var cfg dependencyConfig
	if err := readConfigJSON(in.Check, &cfg); err != nil {
		return nil, err
	}

	deps, err := readManifestDependencies(in.Repo.LocalPath)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for name, version := range deps {
		if bannedMatch(cfg.BannedPackages, name) {
			findings = append(findings, dependencyFinding(in, name, fmt.Sprintf("%s is a banned package", name)))
			continue
		}
		if min, ok := cfg.MinimumVersions[name]; ok && version != "" {
			if cmp, ok := compareVersions(version, min); ok && cmp < 0 {
				findings = append(findings, dependencyFinding(in, name,
					fmt.Sprintf("%s is at %s, below the required minimum %s", name, version, min)))
			}
		}
	}
	return findings, nil
}

func dependencyFinding(in RunInput, target, message string) Finding {
	return Finding{Tool: in.Check.Tool, Repo: in.Repo.Name, Target: target, Message: message, Status: "open"}
}

func bannedMatch(banned []string, name string) bool {
	for _, b := range banned {
		if b == name {
			return true
		}
	}
	return false
}

// readManifestDependencies reads whichever dependency manifest exists
// in dir, returning name → declared version. Only one of the five
// formats is expected per repository (internal/detect.Languages'
// marker files are the same ones checked here); a repository matching
// more than one contributes all of them.
func readManifestDependencies(dir string) (map[string]string, error) {
	deps := map[string]string{}

	if err := readGoModRequires(dir, deps); err != nil {
		return nil, err
	}
	if err := readPackageJSONDeps(dir, deps); err != nil {
		return nil, err
	}
	if err := readPyprojectDeps(dir, deps); err != nil {
		return nil, err
	}
	if err := readCsprojDeps(dir, deps); err != nil {
		return nil, err
	}
	if err := readPomDeps(dir, deps); err != nil {
		return nil, err
	}
	return deps, nil
}

var goRequireLineRE = regexp.MustCompile(`^\s*([^\s]+)\s+v(\S+)`)

// readGoModRequires parses go.mod's require lines (single-line and
// block forms) well enough to extract module → version; it is not a
// full go.mod parser (golang.org/x/mod/modfile would be the real one,
// not pulled in for one field this phase needs from it).
func readGoModRequires(dir string, out map[string]string) error {
	data, ok, err := readFileIfExists(filepath.Join(dir, "go.mod"))
	if err != nil || !ok {
		return err
	}
	inBlock := false
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "require ("):
			inBlock = true
		case inBlock && line == ")":
			inBlock = false
		case inBlock:
			addGoRequireMatch(line, out)
		case strings.HasPrefix(line, "require "):
			addGoRequireMatch(strings.TrimPrefix(line, "require "), out)
		}
	}
	return sc.Err()
}

func addGoRequireMatch(line string, out map[string]string) {
	if m := goRequireLineRE.FindStringSubmatch(line); m != nil {
		out[m[1]] = m[2]
	}
}

type packageJSONShape struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func readPackageJSONDeps(dir string, out map[string]string) error {
	data, ok, err := readFileIfExists(filepath.Join(dir, "package.json"))
	if err != nil || !ok {
		return err
	}
	var pj packageJSONShape
	if err := json.Unmarshal(data, &pj); err != nil {
		return nil //nolint:nilerr // a package.json this loose parse cannot read contributes no dependency data, not a failure
	}
	for name, v := range pj.Dependencies {
		out[name] = stripSemverRangePrefix(v)
	}
	for name, v := range pj.DevDependencies {
		out[name] = stripSemverRangePrefix(v)
	}
	return nil
}

func stripSemverRangePrefix(v string) string {
	return strings.TrimLeft(v, "^~>=< ")
}

var pyprojectDepRE = regexp.MustCompile(`^\s*"([A-Za-z0-9_.-]+)\s*(?:[><=!~]{1,2})?\s*([0-9][0-9A-Za-z.\-]*)?`)

// readPyprojectDeps parses pyproject.toml's [project.dependencies]
// array well enough to extract name → version; not a full TOML parser.
func readPyprojectDeps(dir string, out map[string]string) error {
	data, ok, err := readFileIfExists(filepath.Join(dir, "pyproject.toml"))
	if err != nil || !ok {
		return err
	}
	inDeps := false
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "dependencies") && strings.Contains(line, "["):
			inDeps = true
		case inDeps && strings.Contains(line, "]"):
			inDeps = false
		case inDeps:
			if m := pyprojectDepRE.FindStringSubmatch(line); m != nil {
				out[m[1]] = m[2]
			}
		}
	}
	return sc.Err()
}

var csprojRefRE = regexp.MustCompile(`<PackageReference\s+Include="([^"]+)"\s+Version="([^"]+)"`)

func readCsprojDeps(dir string, out map[string]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil //nolint:nilerr // a directory this checkout does not resolve contributes no dependency data
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".csproj") {
			continue
		}
		data, ok, err := readFileIfExists(filepath.Join(dir, e.Name()))
		if err != nil || !ok {
			continue
		}
		for _, m := range csprojRefRE.FindAllStringSubmatch(string(data), -1) {
			out[m[1]] = m[2]
		}
	}
	return nil
}

var pomDepRE = regexp.MustCompile(`(?s)<dependency>\s*<groupId>([^<]+)</groupId>\s*<artifactId>([^<]+)</artifactId>\s*<version>([^<]+)</version>`)

func readPomDeps(dir string, out map[string]string) error {
	data, ok, err := readFileIfExists(filepath.Join(dir, "pom.xml"))
	if err != nil || !ok {
		return err
	}
	for _, m := range pomDepRE.FindAllStringSubmatch(string(data), -1) {
		out[m[1]+":"+m[2]] = m[3]
	}
	return nil
}

func readFileIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is built from repo.json's own local_path, explicit CLI input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("check: read %s: %w", path, err)
	}
	return data, true, nil
}

// compareVersions compares two dotted-numeric version strings
// (optionally prefixed with a stray "v"), returning -1/0/1 and false
// when either side has a non-numeric component this simple comparator
// cannot order (a pre-release suffix, for instance) — the caller
// treats "cannot compare" as "no finding", never as a violation.
func compareVersions(a, b string) (int, bool) {
	pa, oka := splitNumeric(a)
	pb, okb := splitNumeric(b)
	if !oka || !okb {
		return 0, false
	}
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var va, vb int
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va != vb {
			if va < vb {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

func splitNumeric(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				return nil, false
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out, true
}
