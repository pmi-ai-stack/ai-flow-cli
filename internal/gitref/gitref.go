// Package gitref validates branch and ref names before they reach a git
// argv or a configuration file — a leaf package so internal/config and
// internal/gitops can share one rule without config depending on the
// execution layer.
package gitref

import (
	"fmt"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// Valid reports whether name is usable as a branch or ref name: the
// subset of git check-ref-format's rules that matter for a value that
// reaches argv — non-empty, no leading "-" (it would parse as an
// option), no whitespace or control characters, none of ~^:?*[\, no
// "..", "@{" or "//", and no leading or trailing "/" or ".", no
// trailing ".lock". It returns a usage error naming what to fix, or
// nil.
func Valid(name string) error {
	if reason := invalidRefReason(name); reason != "" {
		return &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("%q is not a valid git branch or ref name: %s", name, reason),
		}
	}
	return nil
}

func invalidRefReason(name string) string {
	switch {
	case name == "":
		return "it is empty"
	case strings.HasPrefix(name, "-"):
		return `it starts with "-"`
	case strings.ContainsAny(name, " ~^:?*[\\\x7f"):
		return `it contains a space or one of ~^:?*[\`
	case strings.Contains(name, ".."), strings.Contains(name, "@{"), strings.Contains(name, "//"):
		return `it contains "..", "@{" or "//"`
	case strings.HasPrefix(name, "/"), strings.HasSuffix(name, "/"),
		strings.HasPrefix(name, "."), strings.HasSuffix(name, "."), strings.HasSuffix(name, ".lock"):
		return `it starts or ends with "/" or ".", or ends with ".lock"`
	}
	for _, r := range name {
		if r < 0x20 {
			return "it contains a control character"
		}
	}
	return ""
}
