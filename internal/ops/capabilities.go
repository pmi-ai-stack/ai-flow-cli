package ops

// Capabilities is which of the seven OPS operations an addon
// implements — the capability matrix's own columns (Render, Validate,
// Drift, DryRun, Rollback, UnitTest, E2ETest), read by `ops addons` and
// by `skills lint`'s capability→section map (phase 10, exercised in
// 29). A capability's absence is not an error anywhere this phase
// touches: `not_applicable` is a value a Drift/DryRun call can return,
// not something Capabilities itself encodes as a reason.
type Capabilities struct {
	Render   bool
	Validate bool
	Drift    bool
	DryRun   bool
	Rollback bool
	UnitTest bool
	E2ETest  bool
}
