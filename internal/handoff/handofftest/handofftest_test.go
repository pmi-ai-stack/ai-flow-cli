// Package handofftest_test exercises handofftest as an external
// caller would — a different package (like any later phase's own
// artifact-writer tests) importing both internal/handoff and
// internal/handoff/handofftest. This is the exact shape internal/
// handoff's own tests cannot use directly (see that package's
// writer_test.go/active_test.go for why), so it is the only place this
// harness gets proven end to end.
package handofftest_test

import (
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff/handofftest"
)

func TestFixedClockPinsAndRestores(t *testing.T) {
	originalBehavior := handoff.Now().Year() // whatever "real now" looks like, for reference only

	t.Run("pinned", func(t *testing.T) {
		fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		handofftest.FixedClock(t, fixed)
		if got := handoff.Now(); !got.Equal(fixed) {
			t.Errorf("handoff.Now() = %v, want %v", got, fixed)
		}
	})
	// The subtest's t.Cleanup already ran by the time it returned, so
	// the clock must be back to producing real wall-clock years here.
	if got := handoff.Now().Year(); got < originalBehavior {
		t.Errorf("handoff.Now() was not restored after the subtest: year = %d", got)
	}
}

func TestRequireByteStablePasses(t *testing.T) {
	result := handofftest.RequireByteStable(t, func() ([]byte, error) {
		return []byte("stable"), nil
	})
	if string(result) != "stable" {
		t.Errorf("result = %q, want %q", result, "stable")
	}
}

// recordingTB wraps a *testing.T so RequireByteStable's failure path
// can be exercised without failing this test itself.
type recordingTB struct {
	testing.TB
	failed bool
}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.failed = true
	panic("requireByteStable: fatal (recorded)")
}

func TestRequireByteStableCatchesADifference(t *testing.T) {
	rt := &recordingTB{TB: t}
	n := 0
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected RequireByteStable to call Fatalf on a difference")
			}
		}()
		handofftest.RequireByteStable(rt, func() ([]byte, error) {
			n++
			return []byte{byte(n)}, nil
		})
	}()
	if !rt.failed {
		t.Error("RequireByteStable did not report the two differing writes")
	}
}
