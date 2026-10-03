package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// toolConfigFileNames is, per check or mutation tool, the configuration
// files that tool picks up from a repository on its own — what an
// agent would edit to make a finding go away instead of fixing it at
// the source, which the guard's row 5 exists to stop. Paths are
// relative to the repository root. A check's own `config` is not here:
// it is a pack path, materialized outside the repository, and a
// repository's own pack lives under .ai-flow/, which row 4 protects
// wholesale. Multipurpose files (pyproject.toml, setup.cfg, pom.xml,
// package.json) are deliberately absent: protecting them would block
// ordinary work for every ticket.
var toolConfigFileNames = map[string][]string{
	"golangci-lint": {".golangci.yml", ".golangci.yaml", ".golangci.toml", ".golangci.json"},
	"eslint": {
		"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", "eslint.config.ts",
		".eslintrc", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.json", ".eslintrc.yml", ".eslintrc.yaml", ".eslintignore",
	},
	"prettier": {
		".prettierrc", ".prettierrc.json", ".prettierrc.yml", ".prettierrc.yaml", ".prettierrc.json5", ".prettierrc.toml",
		".prettierrc.js", ".prettierrc.cjs", ".prettierrc.mjs", "prettier.config.js", "prettier.config.cjs", "prettier.config.mjs",
		".prettierignore",
	},
	"ruff":     {"ruff.toml", ".ruff.toml"},
	"gitleaks": {".gitleaks.toml", ".gitleaksignore"},
	"checkov":  {".checkov.yaml", ".checkov.yml"},
	"trivy":    {"trivy.yaml", ".trivyignore", ".trivyignore.yaml"},
	"zizmor":   {"zizmor.yml", ".github/zizmor.yml"},
	"conftest": {"conftest.toml"},
	"gremlins": {".gremlins.yaml", ".gremlins.yml"},
	"stryker": {
		"stryker.conf.js", "stryker.conf.json", "stryker.conf.mjs", "stryker.conf.cjs",
		"stryker.config.js", "stryker.config.json", "stryker.config.mjs", "stryker.config.cjs",
	},
}

// standardsChecksTools is the subset of standards.json toolConfigFilesFor
// reads.
type standardsChecksTools struct {
	Checks []struct {
		Tool         string  `json:"tool"`
		OverriddenBy *string `json:"overridden_by"`
	} `json:"checks"`
}

// toolConfigFilesFor is active.json's tool_config_files for key: in
// every in-scope repository, each toolConfigFileNames file that exists
// for a tool an effective check names (standards.json) or the
// repository's detected mutation tool uses. Absolute, sorted, deduped.
// Before repo.json or standards.json exists (fetch, locate), it is
// whatever the files that do exist imply — possibly nothing.
func toolConfigFilesFor(home, key string) ([]string, error) {
	ticketDir := handoff.TicketDir(home, key)
	repoArt, err := loadRepoArtifact(ticketDir)
	if err != nil {
		if isMissingArtifact(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var checkTools []string
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "standards.json"))
	if err != nil {
		return nil, err
	}
	if data != nil {
		var st standardsChecksTools
		if err := json.Unmarshal(data, &st); err != nil {
			return nil, fmt.Errorf("cli: decode standards.json: %w", err)
		}
		for _, c := range st.Checks {
			if c.OverriddenBy == nil && c.Tool != "" {
				checkTools = append(checkTools, c.Tool)
			}
		}
	}

	var files []string
	for _, r := range repoArt.Repositories {
		if r.LocalPath == "" {
			continue
		}
		tools := checkTools
		if m, ok, err := detect.Mutation(toDetectRepo(r)); err != nil {
			return nil, err
		} else if ok {
			tools = append(append([]string(nil), checkTools...), m.Tool)
		}
		for _, tool := range strutil.DedupSorted(tools) {
			for _, name := range toolConfigFileNames[tool] {
				p := filepath.Join(r.LocalPath, filepath.FromSlash(name))
				if info, err := os.Stat(p); err == nil && !info.IsDir() {
					abs, err := filepath.Abs(p)
					if err != nil {
						return nil, fmt.Errorf("cli: resolve %s: %w", p, err)
					}
					files = append(files, abs)
				}
			}
		}
	}
	out := strutil.DedupSorted(files)
	if out == nil {
		out = []string{}
	}
	return out, nil
}
