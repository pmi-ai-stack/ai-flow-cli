// Package guard evaluates a normalized agenthost.HookEvent against the
// Guardrails section's seven-row protected table and returns an
// agenthost.Decision — the one thing hook guard needs to turn into a
// host-shaped response. The guard prevents accidents; it is not a
// sandbox, and it fails open by design: a nil Active (no ticket
// active, or a stale one — see the idle rule below) allows every
// ticket-scoped row.
package guard

import (
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// Input bundles everything Evaluate needs: the normalized hook event,
// the current active.json (nil when absent), home (the resolved
// --home directory every path-kind row is rooted at) and the moment
// "now" is evaluated at — injected, not read from handoff.Now()
// directly, so this package stays as pure and as testable as every
// other package in this codebase (07-determinism.md).
type Input struct {
	Event  agenthost.HookEvent
	Active *handoff.Active
	Home   string
	Idle   time.Duration
	Now    time.Time
}

// Evaluate applies the seven-row protected table (rows.go) to in and
// returns the resulting Decision: the first row that matches denies;
// no match allows. The idle rule runs once, up front — "an entry
// whose last_event_at is older than
// config.defaults.guard_idle_hours ... is ignored by the guard",
// which this package reads as "evaluate every row as if Active were
// nil".
//
// Every path — the event's, home, and those active.json lists — is
// normalized first (normalize.go), so no row depends on how a path
// happened to be spelled.
func Evaluate(in Input) agenthost.Decision {
	if in.Active != nil && isStale(*in.Active, in.Idle, in.Now) {
		in.Active = nil
	}
	ev, active, home := normalized(in)

	switch ev.Event {
	case agenthost.EventWrite:
		return evaluateWrite(ev, active, home)
	case agenthost.EventShell:
		return evaluateShell(ev, active)
	default:
		// EventUnknown: a shell/write payload whose tool this adapter did
		// not recognize. EventSessionStart never reaches Evaluate (hook
		// session-start does not call it). Both fall through to the same
		// fail-open default.
		return allowDecision("hook_shape_unknown")
	}
}

func isStale(a handoff.Active, idle time.Duration, now time.Time) bool {
	at, err := time.Parse(time.RFC3339, a.LastEventAt)
	if err != nil {
		// An unparsable timestamp is not this function's problem to fail
		// on — active.json is always written by this binary's own
		// RFC3339 formatter, so a parse failure here means a hand-edited
		// or foreign file, not real staleness.
		return false
	}
	return now.Sub(at) > idle
}

func allowDecision(reason string) agenthost.Decision {
	return agenthost.Decision{Allow: true, Reason: reason}
}

// askDecision is a call the developer must confirm through the host's
// own permission prompt; see agenthost.Decision's Ask.
func askDecision(reason, owning, description string) agenthost.Decision {
	msg := "ai-flow's guard needs your confirmation: " + description + "."
	return agenthost.Decision{
		Ask:           true,
		Reason:        reason,
		OwningCommand: owning,
		UserMessage:   msg,
		AgentMessage:  msg,
	}
}

func denyDecision(reason, owning, description string) agenthost.Decision {
	msg := "ai-flow's guard blocked this: " + description + ". Run `ai-flow " + owning + "` instead."
	return agenthost.Decision{
		Allow:         false,
		Reason:        reason,
		OwningCommand: owning,
		UserMessage:   msg,
		AgentMessage:  msg,
	}
}
