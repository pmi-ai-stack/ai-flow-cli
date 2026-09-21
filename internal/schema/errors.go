package schema

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

// Failure is one leaf validation failure: the JSON pointer into the
// instance that failed, and why.
type Failure struct {
	Pointer string
	Message string
}

// ValidationError is returned by Validate when data does not match
// Artifact's schema. Failures is sorted by (Pointer, Message) so two
// runs over the same invalid input produce the same error text —
// 07-determinism.md's "sort every slice before writing" applies to
// error output exactly as it does to results files.
type ValidationError struct {
	Artifact string
	Failures []Failure
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "schema: %s: %d failure(s)", e.Artifact, len(e.Failures))
	for _, f := range e.Failures {
		fmt.Fprintf(&b, "\n  %s: %s", f.Pointer, f.Message)
	}
	return b.String()
}

// newValidationError flattens the underlying library's nested Causes
// tree into a sorted, flat list of leaf failures.
func newValidationError(artifact string, err error) *ValidationError {
	ve := &ValidationError{Artifact: artifact}

	var walk func(*jsonschema.ValidationError)
	walk = func(node *jsonschema.ValidationError) {
		if node.Message != "" {
			ve.Failures = append(ve.Failures, Failure{
				Pointer: pointerOrRoot(node.InstanceLocation),
				Message: node.Message,
			})
		}
		for _, cause := range node.Causes {
			walk(cause)
		}
	}

	var root *jsonschema.ValidationError
	if !errors.As(err, &root) {
		// Not a *jsonschema.ValidationError at all (a Compile-time or
		// I/O error reaching Validate, which should not happen against
		// an embedded, already-compiled schema) — carry the raw message
		// as the one failure rather than dropping it silently.
		ve.Failures = append(ve.Failures, Failure{Pointer: "", Message: err.Error()})
		return ve
	}
	walk(root)

	slices.SortFunc(ve.Failures, func(a, b Failure) int {
		return cmp.Or(
			cmp.Compare(a.Pointer, b.Pointer),
			cmp.Compare(a.Message, b.Message),
		)
	})
	return ve
}

func pointerOrRoot(p string) string {
	if p == "" {
		return "/"
	}
	return p
}
