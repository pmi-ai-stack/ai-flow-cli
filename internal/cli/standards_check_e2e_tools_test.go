//go:build e2e

package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// The documented CI use case against the rest of the real tool set —
// nightly only (-tags e2e), so a scanner's own flakiness or a
// rule-database update can never block an unrelated PR. Each test
// skips when a tool it needs is absent; the nightly job installs them.

func requireTools(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, ok := exec.Which(tool); !ok {
			t.Skipf("%s not on PATH", tool)
		}
	}
}

// TestE2ETerraformScanFindsWithRealCheckov: a policy-clean module that
// checkov still has opinions about — exit 0, and the should findings
// are reported rather than lost.
func TestE2ETerraformScanFindsWithRealCheckov(t *testing.T) {
	requireTools(t, "conftest", "checkov", "gitleaks")
	repo := filepath.Join(t.TempDir(), "infra-ok")
	writeFileTest(t, filepath.Join(repo, "AGENTS.md"), "# infra\n")
	writeFileTest(t, filepath.Join(repo, "main.tf"), `terraform {
  required_providers {
    aws = { source = "hashicorp/aws" }
  }
}

resource "aws_s3_bucket" "logs" {
  bucket = "example-logs"
  tags   = { team = "platform" }
}
`)

	env := runDirCheck(t, 0, "standards", "check", repo, "--home", t.TempDir())
	var checkov int
	for _, f := range env.Result.Findings {
		if f.CheckID == "infra.terraform.scan" && f.Tool == "checkov" && f.Severity != "must" {
			checkov++
		}
	}
	if checkov == 0 {
		t.Errorf("no checkov findings on an unversioned, unencrypted S3 bucket: %+v", env.Result.Findings)
	}
}

// TestE2EKubernetesManifestsAreReportedSkipped: raw manifests are a
// kubectl target, and every Kubernetes check needs a rendering — so the
// directory form lists them as skipped, never as passed.
func TestE2EKubernetesManifestsAreReportedSkipped(t *testing.T) {
	requireTools(t, "gitleaks")
	repo := filepath.Join(t.TempDir(), "k8s")
	writeFileTest(t, filepath.Join(repo, "AGENTS.md"), "# k8s\n")
	writeFileTest(t, filepath.Join(repo, "deploy", "app.yaml"), "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app\n")

	env := runDirCheck(t, 0, "standards", "check", repo, "--home", t.TempDir())
	var got []string
	for _, s := range env.Result.Skipped {
		got = append(got, s.CheckID)
		if s.Target != "k8s:deploy" || !strings.Contains(s.Reason, "input: render") {
			t.Errorf("skipped entry = %+v", s)
		}
	}
	want := []string{"infra.kubernetes.policy", "infra.kubernetes.scan"}
	if !slices.Equal(got, want) {
		t.Errorf("skipped = %v, want %v", got, want)
	}
}
