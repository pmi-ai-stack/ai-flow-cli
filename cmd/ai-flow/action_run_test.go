package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// actionRun is one run of scripts/action-run.sh (the GitHub Action's
// main step) against a stub ai-flow that records its argv, one per
// line, and prints envelope with exit code exitCode.
type actionRun struct {
	stderr, output, summary string
	argv                    []string
	exitCode                int
}

func runActionScript(t *testing.T, envelope string, exitCode int, inputs map[string]string) actionRun {
	t.Helper()
	sh, ok := exec.Which("sh")
	if !ok {
		t.Skip("sh not on PATH")
	}
	bin := t.TempDir()
	tmp := t.TempDir()
	argvFile := filepath.Join(tmp, "argv")
	envFile := filepath.Join(tmp, "envelope.json")
	if err := os.WriteFile(envFile, []byte(envelope), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > '" + argvFile + "'\ncat '" + envFile + "'\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "ai-flow"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	// The script's own dependencies, from the host: it needs a shell's
	// basic tools, and jq when present.
	for _, tool := range []string{"cat", "jq", "mkdir"} {
		if p, ok := exec.Which(tool); ok {
			if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
		}
	}

	output := filepath.Join(tmp, "github_output")
	summary := filepath.Join(tmp, "step_summary")
	env := []string{
		"PATH=" + bin,
		"AI_FLOW_BIN=" + filepath.Join(bin, "ai-flow"),
		"GITHUB_OUTPUT=" + output,
		"GITHUB_STEP_SUMMARY=" + summary,
		"RUNNER_TEMP=" + tmp,
	}
	for k, v := range inputs {
		env = append(env, k+"="+v)
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "action-run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.NewRunner().RunUnlisted(context.Background(), tmp, sh, []string{script}, env, 30*time.Second)
	if err != nil && !exec.IsToolFailed(err) && out.ExitCode == 0 {
		t.Fatal(err)
	}
	r := actionRun{stderr: out.Stderr, exitCode: out.ExitCode}
	if b, err := os.ReadFile(argvFile); err == nil {
		r.argv = strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	}
	if b, err := os.ReadFile(output); err == nil {
		r.output = string(b)
	}
	if b, err := os.ReadFile(summary); err == nil {
		r.summary = string(b)
	}
	return r
}

func TestActionRunPass(t *testing.T) {
	r := runActionScript(t, `{"ok":true,"command":"standards check","result":{"open":0,"findings":[]},"error":null,"warnings":[]}`, 0,
		map[string]string{"INPUT_PATH": ".", "INPUT_SARIF": "ai-flow.sarif", "INPUT_REQUIRE_INSTRUCTIONS": "false"})
	if r.exitCode != 0 {
		t.Fatalf("exit %d\n%s", r.exitCode, r.stderr)
	}
	if want := []string{"standards", "check", ".", "--sarif", "ai-flow.sarif"}; strings.Join(r.argv, "|") != strings.Join(want, "|") {
		t.Errorf("argv = %q, want %q", r.argv, want)
	}
	if !strings.Contains(r.output, "exit-code=0\n") || !strings.Contains(r.output, "sarif=ai-flow.sarif\n") {
		t.Errorf("GITHUB_OUTPUT = %q", r.output)
	}
	if !strings.Contains(r.summary, "ai-flow standards check") || !strings.Contains(r.summary, "passed") {
		t.Errorf("summary = %q", r.summary)
	}
}

func TestActionRunFailListsEveryDetail(t *testing.T) {
	env := `{"ok":false,"command":"standards check","result":null,"error":{"code":"invalid_result","message":". : 2 open must finding(s)",` +
		`"details":["infra.terraform.policy: main.tf: one","org.secrets: repo: gitleaks is not installed"]},"warnings":[]}`
	r := runActionScript(t, env, 6, map[string]string{
		"INPUT_PATH": "infra", "INPUT_SARIF": "out.sarif", "INPUT_PACK": "https://example.com/pack.git@v1", "INPUT_REQUIRE_INSTRUCTIONS": "true",
	})
	if r.exitCode != 6 {
		t.Fatalf("exit %d, want 6 (the CLI's)\n%s", r.exitCode, r.stderr)
	}
	want := []string{"standards", "check", "infra", "--sarif", "out.sarif", "--pack", "https://example.com/pack.git@v1", "--require-instructions"}
	if strings.Join(r.argv, "|") != strings.Join(want, "|") {
		t.Errorf("argv = %q, want %q", r.argv, want)
	}
	if !strings.Contains(r.output, "exit-code=6\n") {
		t.Errorf("GITHUB_OUTPUT = %q", r.output)
	}
	if _, ok := exec.Which("jq"); ok {
		for _, d := range []string{"infra.terraform.policy: main.tf: one", "org.secrets: repo: gitleaks is not installed", "invalid_result"} {
			if !strings.Contains(r.summary, d) {
				t.Errorf("summary does not list %q:\n%s", d, r.summary)
			}
		}
	}
}

// TestActionRunInputsAreNeverEvaluated: inputs reach argv literally —
// the script quotes every expansion and never evals.
func TestActionRunInputsAreNeverEvaluated(t *testing.T) {
	tmp := t.TempDir()
	pwned := filepath.Join(tmp, "pwned")
	evil := "dir with spaces; touch " + pwned + " $(touch " + pwned + ") `touch " + pwned + "`"
	r := runActionScript(t, `{"ok":true,"command":"standards check","result":{"open":0,"findings":[]},"error":null,"warnings":[]}`, 0,
		map[string]string{"INPUT_PATH": evil, "INPUT_SARIF": "a b.sarif", "INPUT_REQUIRE_INSTRUCTIONS": "false"})
	if len(r.argv) < 5 || r.argv[2] != evil || r.argv[4] != "a b.sarif" {
		t.Errorf("argv = %q, want the inputs verbatim", r.argv)
	}
	if _, err := os.Stat(pwned); err == nil {
		t.Error("an input was evaluated by the shell")
	}
}
