package kustomize

import "github.com/pmi-ai-stack/ai-flow-cli/internal/ops"

// kindClasses maps a Kubernetes kind to its blast-radius KindClass —
// identical to kubectl's own table (Kubernetes kinds are Kubernetes
// kinds regardless of which addon rendered them; see
// .ai/phase-26/plan.md §0.11 for why this is a duplicate, not a shared
// package). Unmatched kinds default to KindOther, not KindWorkload —
// the same divergence from terraform's own default kubectl's own
// kindclass.go already documents.
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

// classifyKind maps kind to a KindClass by exact match, defaulting to
// KindOther.
func classifyKind(kind string) ops.KindClass {
	if c, ok := kindClasses[kind]; ok {
		return c
	}
	return ops.KindOther
}

// rolloutCapable reports whether kind supports "kubectl rollout
// history"/"rollout undo".
func rolloutCapable(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return true
	default:
		return false
	}
}
