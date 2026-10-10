package ops

import "slices"

// KindClass is an action's blast-radius classification — assigned per
// addon from a table (the terraform/kubectl kind-class maps arrive in
// phases 21/22), consumed by risk scoring (phase 23). The nine values
// are exactly the spec's own enum (§ Risk scoring); nothing here may
// add a tenth without a spec PR first.
type KindClass string

const (
	KindWorkload       KindClass = "workload"
	KindNetwork        KindClass = "network"
	KindRBAC           KindClass = "rbac"
	KindSecret         KindClass = "secret"
	KindData           KindClass = "data"
	KindIAM            KindClass = "iam"
	KindCRDOrNamespace KindClass = "crd_or_namespace"
	KindPipeline       KindClass = "pipeline"
	KindOther          KindClass = "other"
)

// kindClasses is every valid KindClass, in the spec's own declared
// order — IsKindClass's backing data.
var kindClasses = []KindClass{
	KindWorkload, KindNetwork, KindRBAC, KindSecret, KindData,
	KindIAM, KindCRDOrNamespace, KindPipeline, KindOther,
}

// IsKindClass reports whether k is one of the nine documented values.
func IsKindClass(k KindClass) bool {
	return slices.Contains(kindClasses, k)
}
