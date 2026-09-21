// Package handofftest is the reusable byte-stability harness every
// writer built on internal/handoff (and, from phase 4 on, every other
// package's own artifact writer) proves itself against: a fixed clock,
// and an assertion that identical inputs produce identical bytes.
package handofftest

import (
	"bytes"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// FixedClock pins handoff.Now to at for the duration of the test,
// restoring the original clock on cleanup.
func FixedClock(tb testing.TB, at time.Time) {
	tb.Helper()
	orig := handoff.Now
	handoff.Now = func() time.Time { return at }
	tb.Cleanup(func() { handoff.Now = orig })
}

// RequireByteStable calls write twice and fails tb if the two results
// differ, proving identical inputs produce identical bytes
// (07-determinism.md). It returns the (single, since both calls agreed)
// result for further assertions.
func RequireByteStable(tb testing.TB, write func() ([]byte, error)) []byte {
	tb.Helper()
	first, err := write()
	if err != nil {
		tb.Fatalf("handofftest: first write: %v", err)
	}
	second, err := write()
	if err != nil {
		tb.Fatalf("handofftest: second write: %v", err)
	}
	if !bytes.Equal(first, second) {
		tb.Fatalf("handofftest: not byte-stable:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	return first
}
