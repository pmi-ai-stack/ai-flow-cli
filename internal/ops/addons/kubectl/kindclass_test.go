package kubectl

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
		{"CustomResourceDefinition", ops.KindCRDOrNamespace},
		{"Secret", ops.KindSecret},
		{"ServiceAccount", ops.KindRBAC},
		{"Role", ops.KindRBAC},
		{"RoleBinding", ops.KindRBAC},
		{"ClusterRole", ops.KindRBAC},
		{"ClusterRoleBinding", ops.KindRBAC},
		{"PersistentVolume", ops.KindData},
		{"PersistentVolumeClaim", ops.KindData},
		{"Service", ops.KindNetwork},
		{"Ingress", ops.KindNetwork},
		{"NetworkPolicy", ops.KindNetwork},
		{"Endpoints", ops.KindNetwork},
		{"EndpointSlice", ops.KindNetwork},
		{"Deployment", ops.KindWorkload},
		{"StatefulSet", ops.KindWorkload},
		{"DaemonSet", ops.KindWorkload},
		{"ReplicaSet", ops.KindWorkload},
		{"Pod", ops.KindWorkload},
		{"Job", ops.KindWorkload},
		{"CronJob", ops.KindWorkload},
		{"ReplicationController", ops.KindWorkload},
		{"ConfigMap", ops.KindOther},
		{"StorageClass", ops.KindOther},
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
	for _, k := range []string{"Pod", "Job", "Service", "ConfigMap"} {
		if rolloutCapable(k) {
			t.Errorf("rolloutCapable(%q) = true, want false", k)
		}
	}
}
