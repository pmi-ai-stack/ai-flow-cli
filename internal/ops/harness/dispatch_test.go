package harness_test

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/harness"
)

func TestToolForDispatch(t *testing.T) {
	cases := []struct {
		name     string
		addon    string
		h        ops.Harness
		wantTool string
		wantOK   bool
	}{
		{"terraform unit", "terraform", ops.Harness{Name: "modules/net/main.tftest.hcl", Kind: ops.HarnessUnit}, "terraform-test", true},
		{"terraform e2e terratest", "terraform", ops.Harness{Name: "modules/net/sandbox_test.go", Kind: ops.HarnessE2E}, "terratest", true},
		{"kubectl kuttl", "kubectl", ops.Harness{Name: "manifests/kuttl-test.yaml", Kind: ops.HarnessE2E}, "kuttl", true},
		{"kubectl chainsaw", "kubectl", ops.Harness{Name: "manifests/chainsaw-test.yaml", Kind: ops.HarnessE2E}, "chainsaw", true},
		{"kustomize reuses kuttl convention", "kustomize", ops.Harness{Name: "overlays/prod/kuttl-test.yaml", Kind: ops.HarnessE2E}, "kuttl", true},
		{"terraform unit misclassified as e2e is not terraform-test", "terraform", ops.Harness{Name: "x.tftest.hcl", Kind: ops.HarnessE2E}, "terratest", true},
		{"unknown addon, no matching file name", "helm", ops.Harness{Name: "values.yaml", Kind: ops.HarnessE2E}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool, ok := harness.ToolFor(tc.addon, tc.h)
			if tool != tc.wantTool || ok != tc.wantOK {
				t.Errorf("ToolFor(%q, %+v) = (%q, %v), want (%q, %v)", tc.addon, tc.h, tool, ok, tc.wantTool, tc.wantOK)
			}
		})
	}
}

func TestNeedsCluster(t *testing.T) {
	if !harness.NeedsCluster("kuttl") || !harness.NeedsCluster("chainsaw") {
		t.Error("kuttl and chainsaw both need the kind cluster")
	}
	if harness.NeedsCluster("terraform-test") || harness.NeedsCluster("terratest") {
		t.Error("terraform's own two tools never need the kind cluster")
	}
}
