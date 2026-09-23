package ops

// DriftKind is render-drift-results.json's own "drift" enum.
type DriftKind string

const (
	DriftNone          DriftKind = "none"
	DriftPresent       DriftKind = "present"
	DriftNotApplicable DriftKind = "not_applicable"
	DriftOffline       DriftKind = "offline"
)

// DriftReport is Addon.Drift's result: the base rendering compared
// against the live environment, so pre-existing drift is separated
// from the ticket's own change (phase 20 owns the comparison itself).
// Fields are declared in JSON-key alphabetical order (target.go's own
// documented byte-stability convention).
type DriftReport struct {
	Drift     DriftKind `json:"drift"`
	Resources []string  `json:"drift_resources"`
}
