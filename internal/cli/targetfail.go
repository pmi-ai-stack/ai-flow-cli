package cli

import (
	"errors"
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// targetFailures collects what went wrong per target so a multi-target
// command (`ops render`, `ops drift`, `ops dry-run`) can attempt every
// target and report every failure in one run, instead of stopping at the
// first and hiding the rest. Failures are kept in the order the targets
// were attempted — a function of the target list, never of scheduling.
//
// It deliberately does not become a field of the results artifacts:
// those are trusted state later stages read, so a half-computed one must
// not exist. A run with any failure fails and writes none.
type targetFailures struct {
	items []targetFailure
}

type targetFailure struct {
	target ops.Target
	err    error
}

func (tf *targetFailures) add(t ops.Target, err error) {
	tf.items = append(tf.items, targetFailure{target: t, err: err})
}

// first is the first failure's own error, for callers that derive more
// than the envelope error from it (dry-run's needs_input).
func (tf *targetFailures) first() error {
	if len(tf.items) == 0 {
		return nil
	}
	return tf.items[0].err
}

// err is nil when nothing failed. With a single target in the run the
// original error is returned untouched, so single-target behaviour is
// byte-identical. Otherwise: the first failure's code (so the exit code
// and its meaning are kept), "N of M targets failed", and one detail
// line per failure.
func (tf *targetFailures) err(total int) error {
	switch len(tf.items) {
	case 0:
		return nil
	case 1:
		if total <= 1 {
			return tf.items[0].err
		}
	}
	details := make([]string, 0, len(tf.items))
	for _, f := range tf.items {
		code, msg := "internal", f.err.Error()
		var e *envelope.Error
		if errors.As(f.err, &e) {
			code, msg = e.Code, e.Message
		}
		details = append(details, fmt.Sprintf("%s: %s: %s", ops.TargetKey(f.target), code, msg))
	}
	code := "internal"
	var e *envelope.Error
	if errors.As(tf.items[0].err, &e) {
		code = e.Code
	}
	return &envelope.Error{
		Code:    code,
		Message: fmt.Sprintf("%d of %d targets failed", len(tf.items), total),
		Details: details,
	}
}
