package ops

// HarnessKind is one harness's own kind — unit harnesses always run;
// e2e harnesses only when ops.e2e.enabled and a sandbox binding exists
// (phase 24 owns the lifecycle).
type HarnessKind string

const (
	HarnessUnit HarnessKind = "unit"
	HarnessE2E  HarnessKind = "e2e"
)

// Harness is one Addon.Harnesses result — a runnable test the
// infra-test stage detects and runs (phase 24).
type Harness struct {
	Name string
	Kind HarnessKind
}
