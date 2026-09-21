package handoff

// State is one of the seven states in the stage state machine.
type State string

const (
	StatePending          State = "pending"
	StateRunning          State = "running"
	StateAwaitingApproval State = "awaiting_approval"
	StatePass             State = "pass"
	StateFail             State = "fail"
	StateSkipped          State = "skipped"
	StateApproved         State = "approved"
)

// IsTerminal reports whether s is one of the four states stage reset
// may start from: fail, pass, skipped, approved. pending, running and
// awaiting_approval are all "in progress" and are not reset targets —
// there is nothing to reset from pending, and resetting mid-flight
// would discard work reset is not meant to discard.
func IsTerminal(s State) bool {
	switch s {
	case StateFail, StatePass, StateSkipped, StateApproved:
		return true
	default:
		return false
	}
}
