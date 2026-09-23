package standards

import (
	"bytes"
	"fmt"
	"strings"
)

// GuidanceFile is the artifact this package renders into a ticket's
// handoff directory.
const GuidanceFile = "guidance.md"

// RenderGuidance produces guidance.md for one stage: a header naming
// the packs and their commits, each selected section under
// `## [must|should|may] <id>`, then the `Dropped by budget:` line —
// the Guidance delivery section's own layout.
//
// Pure: the same Effective and the same stage always produce the same
// bytes. No clock value appears anywhere in the output, which is what
// makes `stage start` re-runnable byte for byte.
func RenderGuidance(e *Effective, key, stage string) ([]byte, error) {
	kept, dropped, mustOverflow := e.SectionsForStage(stage)

	var b bytes.Buffer
	fmt.Fprintf(&b, "# Guidance — %s · %s\n\n", key, stage)
	fmt.Fprintf(&b, "Packs: %s\n", packLine(e.Packs))
	fmt.Fprintf(&b, "Languages: %s\n", listOrNone(e.Languages))
	fmt.Fprintf(&b, "Addons: %s\n", listOrNone(e.Addons))
	fmt.Fprintf(&b, "Budget: %d tokens\n", e.BudgetTokens)
	if mustOverflow > 0 {
		// The spec's own wording, verbatim: every must section is still
		// carried below, and nothing is truncated — only the overflow is
		// recorded, here and in doctor.
		fmt.Fprintf(&b, "Budget exceeded by must sections: %d bytes\n", mustOverflow)
	}
	b.WriteString("\n")

	for _, s := range kept {
		fmt.Fprintf(&b, "## [%s] %s\n\n", s.Severity, s.ID)
		body, err := s.Body()
		if err != nil {
			return nil, err
		}
		b.Write(body)
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Dropped by budget: %s\n", listOrNone(dropped))
	return b.Bytes(), nil
}

// normalizeBody trims a section file's surrounding whitespace and
// re-terminates it with exactly one newline, so the blank line before
// the next heading is contributed by the renderer rather than by
// whatever the pack author's editor happened to leave behind. This is
// also what the budget measures, so the measurement and the rendering
// can never disagree about a section's size.
func normalizeBody(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return []byte("\n")
	}
	return append(trimmed, '\n')
}

// packLine renders the header's "Packs:" value: `id@ref (commit)` per
// pack, in layer order. A pack with no commit at all — a local-path
// source, a repository's own in-tree pack — shows its fingerprint's
// short form instead, so every pack in the line is identified by
// something and none is silently blank.
func packLine(packs []*Pack) string {
	if len(packs) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(packs))
	for _, p := range packs {
		parts = append(parts, fmt.Sprintf("%s@%s (%s)", p.ID, p.Ref, shortID(p)))
	}
	return strings.Join(parts, ", ")
}

// shortID is a pack's commit, shortened to the usual 8 hex characters,
// or its fingerprint's own short form when it has no commit.
func shortID(p *Pack) string {
	if p.Commit != "" {
		return short(p.Commit)
	}
	return short(strings.TrimPrefix(p.Fingerprint, "sha256:"))
}

func short(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}

// listOrNone renders a string list for the header, or the literal
// "(none)" — never an empty value, so every line in a guidance.md
// header carries the same shape whether or not there is anything to
// say.
func listOrNone(in []string) string {
	if len(in) == 0 {
		return "(none)"
	}
	return strings.Join(in, ", ")
}
