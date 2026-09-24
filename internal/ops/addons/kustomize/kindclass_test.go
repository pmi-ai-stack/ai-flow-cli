package kustomize

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		kind string
		want ops.KindClass
	}{
		{"Namespace", ops.KindCRDOrNamespace},
		{"Secret", ops.KindSecret},
		{"ServiceAccount", ops.KindRBAC},
		{"PersistentVolumeClaim", ops.KindData},
		{"Service", ops.KindNetwork},
		{"Deployment", ops.KindWorkload},
		{"ConfigMap", ops.KindOther},
		{"MyCustomResource", ops.KindOther},
	}
	for _, c := range cases {
		if got := classifyKind(c.kind); got != c.want {
			t.Errorf("classifyKind(%q) = %q, want %q", c.kind, got, c.want)
		}
	}
}

func TestRolloutCapable(t *testing.T) {
	for _, k := range []string{"Deployment", "StatefulSet", "DaemonSet"} {
		if !rolloutCapable(k) {
			t.Errorf("rolloutCapable(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"Pod", "Service", "ConfigMap"} {
		if rolloutCapable(k) {
			t.Errorf("rolloutCapable(%q) = true, want false", k)
		}
	}
}
