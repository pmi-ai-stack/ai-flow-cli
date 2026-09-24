package kubectl

import "github.com/pmi-ai-stack/ai-flow-cli/internal/ops"

// kindClasses maps a Kubernetes kind to its blast-radius KindClass.
// Unmatched kinds (ConfigMap, StorageClass, an unrecognized CRD
// instance, ...) default to KindOther, not KindWorkload — a deliberate
// divergence from terraform's own addon (.ai/phase-22/plan.md design
// decision 3): an unrecognized Kubernetes kind is overwhelmingly a
// custom resource defined by some operator's CRD, far more plausibly
// configuration/glue than a running workload.
var kindClasses = map[string]ops.KindClass{
	"Namespace":                ops.KindCRDOrNamespace,
	"CustomResourceDefinition": ops.KindCRDOrNamespace,

	"Secret": ops.KindSecret,

	"ServiceAccount":     ops.KindRBAC,
	"Role":               ops.KindRBAC,
	"RoleBinding":        ops.KindRBAC,
	"ClusterRole":        ops.KindRBAC,
	"ClusterRoleBinding": ops.KindRBAC,

	"PersistentVolume":      ops.KindData,
	"PersistentVolumeClaim": ops.KindData,

	"Service":       ops.KindNetwork,
	"Ingress":       ops.KindNetwork,
	"NetworkPolicy": ops.KindNetwork,
	"Endpoints":     ops.KindNetwork,
	"EndpointSlice": ops.KindNetwork,

	"Deployment":            ops.KindWorkload,
	"StatefulSet":           ops.KindWorkload,
	"DaemonSet":             ops.KindWorkload,
	"ReplicaSet":            ops.KindWorkload,
	"Pod":                   ops.KindWorkload,
	"Job":                   ops.KindWorkload,
	"CronJob":               ops.KindWorkload,
	"ReplicationController": ops.KindWorkload,
}

// classifyKind maps kind (a manifest's own "kind" field, e.g.
// "Deployment") to a KindClass by exact match, defaulting to
// KindOther.
func classifyKind(kind string) ops.KindClass {
	if c, ok := kindClasses[kind]; ok {
		return c
	}
	return ops.KindOther
}

// rolloutCapable reports whether kind supports "kubectl rollout
// history"/"rollout undo" — Rollback's own signal for which resources
// it can produce a real, tool-backed rollback instruction for.
func rolloutCapable(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return true
	default:
		return false
	}
}
