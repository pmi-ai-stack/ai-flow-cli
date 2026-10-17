package check_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

// scratchCase is one runner that writes a file of its own: a config, a
// policy directory or a report.
type scratchCase struct {
	name  string
	kind  string
	check standards.EffectiveCheck
	in    func(scratch string) check.RunInput
	// paths are the files the runner must have written under scratch by
	// the time the tool runs, relative to scratch.
	paths []string
}

func scratchCases() []scratchCase {
	repo := check.Repo{Name: "acme.api", LocalPath: "/repo"}
	return []scratchCase{
		{
			name: "secrets", kind: "secrets",
			check: standards.EffectiveCheck{
				ID: "org.secrets", Tool: "gitleaks", Severity: "must", Config: "gitleaks.toml",
				Pack: packWithConfig("gitleaks.toml", "title = \"test\"\n"),
			},
			in:    func(s string) check.RunInput { return check.RunInput{Repo: repo, Scratch: s} },
			paths: []string{"config-org.secrets.toml"},
		},
		{
			name: "format config", kind: "format",
			check: standards.EffectiveCheck{
				ID: "python.format", Tool: "ruff", Severity: "must", Config: "ruff.toml",
				Pack: packWithConfig("ruff.toml", "line-length = 100\n"),
			},
			in:    func(s string) check.RunInput { return check.RunInput{Repo: repo, Scratch: s} },
			paths: []string{"config-python.format.toml"},
		},
		{
			name: "policy dir", kind: "policy",
			check: standards.EffectiveCheck{
				ID: "infra.terraform.policy", Kind: "policy", Tool: "conftest", Severity: "must",
				Config: "policy", Pack: packWithConfigDir("policy", map[string]string{"naming.rego": "package main\n"}),
			},
			in: func(s string) check.RunInput {
				return check.RunInput{
					Target:  check.Target{Key: "acme.infra:.", Addon: "terraform", Repo: "acme.infra", SourceDir: "/repo"},
					Scratch: s,
				}
			},
			paths: []string{filepath.Join("policy-infra.terraform.policy", "naming.rego")},
		},
	}
}

// TestRunnersRefuseEmptyScratch: a runner that has to write a file and
// was given no private directory fails before running anything. It
// never falls back to a shared, predictable path in os.TempDir().
func TestRunnersRefuseEmptyScratch(t *testing.T) {
	for _, tc := range scratchCases() {
		t.Run(tc.name, func(t *testing.T) {
			runner, _ := check.ForKind(tc.kind)
			in := tc.in("")
			in.Check = tc.check
			_, err := runner.Run(context.Background(), in, exectest.New(t), time.Second)
			if err == nil || !strings.Contains(err.Error(), "scratch") {
				t.Fatalf("err = %v, want an error naming the missing scratch directory", err)
			}
		})
	}
}

// TestRunnersWriteOnlyUnderScratch: every file a runner writes lives in
// RunInput.Scratch while the tool runs, nothing lands in os.TempDir(),
// and the runner removes its files afterwards.
func TestRunnersWriteOnlyUnderScratch(t *testing.T) {
	for _, tc := range scratchCases() {
		t.Run(tc.name, func(t *testing.T) {
			scratch := t.TempDir()
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)

			var seen []string
			x := execFunc(func(_ string, args []string) (exec.Output, error) {
				for _, p := range tc.paths {
					if _, err := os.Stat(filepath.Join(scratch, p)); err == nil {
						seen = append(seen, p)
					}
				}
				for _, a := range args {
					if strings.HasPrefix(a, tmp) {
						t.Errorf("argument %q is under os.TempDir()", a)
					}
				}
				return exec.Output{}, nil
			})

			runner, _ := check.ForKind(tc.kind)
			in := tc.in(scratch)
			in.Check = tc.check
			if _, err := runner.Run(context.Background(), in, x, time.Second); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(seen) != len(tc.paths) {
				t.Errorf("files present while the tool ran = %v, want %v", seen, tc.paths)
			}
			for _, dir := range []string{tmp, scratch} {
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Errorf("%s holds %d entries after the run, want none (first: %s)", dir, len(entries), entries[0].Name())
				}
			}
		})
	}
}

// execFunc adapts a function to exec.Execer, checking the argv against
// the allowlist exactly like exectest.Scripted does.
type execFunc func(tool string, args []string) (exec.Output, error)

func (f execFunc) Run(_ context.Context, tool string, args []string, _ exec.RunOpts) (exec.Output, error) {
	if err := exectest.Refused(tool, args); err != nil {
		return exec.Output{}, err
	}
	return f(tool, args)
}
