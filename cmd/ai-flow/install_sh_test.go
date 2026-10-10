package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// stubInstallTools writes uname, curl and tar stubs into a fresh
// directory: uname reports osName, curl records that it was called and
// fails, tar fails. install.sh must decide about the OS before it
// downloads anything.
func stubInstallTools(t *testing.T, osName string) (bin, curlLog string) {
	t.Helper()
	bin = t.TempDir()
	curlLog = filepath.Join(t.TempDir(), "curl.log")
	stubs := map[string]string{
		"uname": "#!/bin/sh\nif [ \"$1\" = -m ]; then echo x86_64; else echo '" + osName + "'; fi\n",
		"curl":  "#!/bin/sh\necho \"$@\" >> '" + curlLog + "'\nexit 22\n",
		"tar":   "#!/bin/sh\nexit 1\n",
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return bin, curlLog
}

// runInstallSh runs the repository's own install.sh under sh with only
// bin on PATH. It goes through exec's unlisted runner: install.sh is
// not a tool the CLI runs, so it has no allowlist row, and this module
// runs no subprocess outside internal/ops/exec.
func runInstallSh(t *testing.T, bin string) (stderr string, exitCode int) {
	t.Helper()
	sh, ok := exec.Which("sh")
	if !ok {
		t.Skip("sh not on PATH")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin, "HOME=" + t.TempDir()}
	out, err := exec.NewRunner().RunUnlisted(context.Background(), t.TempDir(), sh,
		[]string{script, "-v", "v9.9.9", "-d", t.TempDir()}, env, 30*time.Second)
	if err != nil && !exec.IsToolFailed(err) && out.ExitCode == 0 {
		t.Fatal(err)
	}
	return out.Stderr, out.ExitCode
}

// TestInstallShPointsWindowsAtTheZip: under Git Bash, MSYS or Cygwin,
// install.sh does not install, but names the release's Windows archive
// and the README section that installs it — before downloading
// anything (phase 50, FEAT-9).
func TestInstallShPointsWindowsAtTheZip(t *testing.T) {
	for _, osName := range []string{"MINGW64_NT-10.0-19045", "MSYS_NT-10.0-19045", "CYGWIN_NT-10.0-19045"} {
		t.Run(osName, func(t *testing.T) {
			bin, curlLog := stubInstallTools(t, osName)
			stderr, ec := runInstallSh(t, bin)
			if ec != 1 {
				t.Errorf("exit code = %d, want 1\n%s", ec, stderr)
			}
			for _, want := range []string{"ai-flow_9.9.9_windows_amd64.zip", "README", "Windows"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr does not mention %q:\n%s", want, stderr)
				}
			}
			if _, err := os.Stat(curlLog); err == nil {
				t.Error("install.sh called curl before refusing the OS")
			}
		})
	}
}

// TestInstallShRefusesUnknownOS keeps the generic refusal for every
// other OS.
func TestInstallShRefusesUnknownOS(t *testing.T) {
	bin, curlLog := stubInstallTools(t, "SunOS")
	stderr, ec := runInstallSh(t, bin)
	if ec != 1 || !strings.Contains(stderr, "unsupported OS: SunOS") {
		t.Errorf("exit %d, stderr:\n%s", ec, stderr)
	}
	if _, err := os.Stat(curlLog); err == nil {
		t.Error("install.sh called curl before refusing the OS")
	}
}
