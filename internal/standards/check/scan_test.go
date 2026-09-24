package check_test

import (
	"context"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

func TestRunScanCheckovSingleFrameworkObject(t *testing.T) {
	report := `{"results":{"failed_checks":[{"check_id":"CKV_AWS_20","check_name":"S3 bucket is public",` +
		`"severity":"HIGH","file_path":"/main.tf","file_line_range":[10,20],"resource":"aws_s3_bucket.foo"}]}}`
	x := exectest.New(t).On("checkov", []string{"--directory", ".", "--output", "json", "--compact", "--quiet"}, exec.Output{Stdout: report})

	c := standards.EffectiveCheck{ID: "infra.terraform.scan", Kind: "scan", Tool: "checkov", Severity: "should"}
	runner, ok := check.ForKind("scan")
	if !ok {
		t.Fatal("no scan runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Target: check.Target{Key: "acme.infra:modules/vpc", Repo: "acme.infra", SourceDir: "/repo/modules/vpc"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1", findings)
	}
	f := findings[0]
	if f.File != "main.tf" || f.Line != 10 {
		t.Errorf("file/line = %q/%d", f.File, f.Line)
	}
	if f.Message != "[CKV_AWS_20] S3 bucket is public (aws_s3_bucket.foo)" {
		t.Errorf("message = %q", f.Message)
	}
	want := check.CapSeverity("high", c.Severity)
	if f.Severity != want {
		t.Errorf("severity = %q, want %q", f.Severity, want)
	}
}

func TestRunScanCheckovMultiFrameworkArray(t *testing.T) {
	report := `[{"results":{"failed_checks":[{"check_id":"CKV_K8S_1","check_name":"no privileged","severity":null,"file_path":"pod.yaml"}]}}]`
	x := exectest.New(t).On("checkov", []string{"--directory", ".", "--output", "json", "--compact", "--quiet"}, exec.Output{Stdout: report})

	c := standards.EffectiveCheck{ID: "infra.terraform.scan", Kind: "scan", Tool: "checkov", Severity: "should"}
	runner, _ := check.ForKind("scan")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Target: check.Target{Key: "acme.infra:modules/vpc", Repo: "acme.infra", SourceDir: "/repo/modules/vpc"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "pod.yaml" {
		t.Fatalf("findings = %#v, want one for pod.yaml", findings)
	}
}

func TestRunScanCheckovClean(t *testing.T) {
	x := exectest.New(t).On("checkov", []string{"--directory", ".", "--output", "json", "--compact", "--quiet"}, exec.Output{Stdout: `{"results":{"failed_checks":[]}}`})
	c := standards.EffectiveCheck{ID: "infra.terraform.scan", Kind: "scan", Tool: "checkov", Severity: "should"}
	runner, _ := check.ForKind("scan")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Target: check.Target{Key: "acme.infra:modules/vpc", Repo: "acme.infra", SourceDir: "/repo/modules/vpc"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestRunScanTrivyConfig(t *testing.T) {
	report := `{"Results":[{"Target":"deploy.yaml","Misconfigurations":[{"ID":"AVD-KSV-0001","Title":"Root filesystem",` +
		`"Message":"container should not run as root","Severity":"CRITICAL","CauseMetadata":{"StartLine":5}}]}]}`
	x := exectest.New(t).On("trivy", []string{"config", "--format", "json", "."}, exec.Output{Stdout: report})

	c := standards.EffectiveCheck{ID: "infra.kubernetes.scan", Kind: "scan", Tool: "trivy", Severity: "should", Input: "render"}
	runner, _ := check.ForKind("scan")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Target: check.Target{Key: "acme.k8s:overlays/prod", Repo: "acme.k8s", RenderDir: "/ticket/render/x"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1", findings)
	}
	f := findings[0]
	if f.File != "deploy.yaml" || f.Line != 5 {
		t.Errorf("file/line = %q/%d", f.File, f.Line)
	}
	want := check.CapSeverity("critical", c.Severity)
	if f.Severity != want {
		t.Errorf("severity = %q, want %q", f.Severity, want)
	}
}

func TestRunScanKyvernoApply(t *testing.T) {
	report := `{"results":[{"policy":"require-labels","rule":"check-team-label","message":"label 'team' is required",` +
		`"result":"fail","severity":"medium","resources":[{"name":"web-deployment"}]}]}`
	fake := &conftestExecer{Scripted: exectest.New(t), out: exec.Output{Stdout: report}, tool: "kyverno"}

	c := standards.EffectiveCheck{
		ID: "infra.kubernetes.kyverno", Kind: "scan", Tool: "kyverno", Severity: "must", Input: "render",
		Config: "policy", Pack: packWithConfigDir("policy", map[string]string{"require-labels.yaml": "kind: ClusterPolicy\n"}),
	}
	runner, _ := check.ForKind("scan")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Target: check.Target{Key: "acme.k8s:overlays/prod", Repo: "acme.k8s", RenderDir: "/ticket/render/x"},
	}, fake, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1", findings)
	}
	f := findings[0]
	if f.File != "web-deployment" || f.Message != "[require-labels/check-team-label] label 'team' is required" {
		t.Errorf("finding = %+v", f)
	}
	if len(fake.calls) != 1 || fake.calls[0].dir != "/ticket/render/x" {
		t.Errorf("kyverno call = %+v, want one call scoped to the render dir", fake.calls)
	}
}

func TestRunScanKyvernoApplyNoConfigIsError(t *testing.T) {
	c := standards.EffectiveCheck{ID: "x", Kind: "scan", Tool: "kyverno", Severity: "must"}
	runner, _ := check.ForKind("scan")
	_, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Target: check.Target{Key: "a:b", RenderDir: "/x"},
	}, exectest.New(t), time.Second)
	if err == nil {
		t.Fatal("Run: want an error when kyverno has no Config, got nil")
	}
}

func TestRunScanUnknownToolIsError(t *testing.T) {
	c := standards.EffectiveCheck{ID: "x", Kind: "scan", Tool: "snyk", Severity: "must"}
	runner, _ := check.ForKind("scan")
	_, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Target: check.Target{Key: "a:b", SourceDir: "/repo"},
	}, exectest.New(t), time.Second)
	if err == nil {
		t.Fatal("Run: want an error for an unrecognized scan tool, got nil")
	}
}
