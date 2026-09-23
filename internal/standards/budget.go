package standards

import (
	"cmp"
	"slices"
)

// severityRank and layerRank are the budget's two ordering keys, in
// the spec's own order: `must → should → may`, then layer
// `repo → team → org`, then id. An unknown value sorts last rather
// than panicking — the pack schema already constrains both fields, so
// an unknown one here would be a bug upstream, not caller input.
func severityRank(s string) int {
	switch s {
	case "must":
		return 0
	case "should":
		return 1
	case "may":
		return 2
	default:
		return 3
	}
}

func layerRank(l string) int {
	switch l {
	case LayerRepo:
		return 0
	case LayerTeam:
		return 1
	case LayerOrg:
		return 2
	default:
		return 3
	}
}

// precedenceOrder is the one total order sections are selected,
// rendered and written in: `must → should → may`, then
// `repo → team → org`, then id. Taken as loose fields rather than a
// struct so both callers — the budget, over budgetCandidate, and
// applyPrecedence, over EffectiveSection — can use it without either
// building a throwaway value of the other's type.
func precedenceOrder(severityA, layerA, idA, severityB, layerB, idB string) int {
	return cmp.Or(
		cmp.Compare(severityRank(severityA), severityRank(severityB)),
		cmp.Compare(layerRank(layerA), layerRank(layerB)),
		cmp.Compare(idA, idB),
	)
}

// budgetOrder is precedenceOrder over two candidates.
func budgetOrder(a, b budgetCandidate) int {
	return precedenceOrder(a.severity, a.layer, a.id, b.severity, b.layer, b.id)
}

// budgetCandidate is one section competing for the stage's budget.
type budgetCandidate struct {
	id       string
	layer    string
	severity string
	// bytes is the length of the section body as it will appear in
	// guidance.md — the only measurement that makes the budget describe
	// the artifact it is budgeting.
	bytes int
}

// tokensFor is the spec's own approximation: "tokens are approximated
// as bytes/4 so the selection is byte-stable." Integer division, so
// the result is a pure function of the bytes and nothing else.
func tokensFor(b int) int { return b / 4 }

// budgetSelection is selectForStage's result.
type budgetSelection struct {
	Kept    []string
	Dropped []string
	// MustOverflowBytes is how far the `must` set alone exceeded the
	// budget. Nothing truncates a must; this records the overflow so
	// guidance.md and doctor can report it, which is the spec's own
	// documented behaviour.
	MustOverflowBytes int
}

// selectForStage applies the token budget to one stage's applicable
// sections.
//
// Sections are ordered `must → should → may`, then `repo → team → org`,
// then id. Every `must` is kept unconditionally; `should` and `may` are
// then taken in that same order while they fit. Because the order is
// total and the arithmetic is integer, the selection is a pure function
// of its inputs — the byte-stability the spec asks for.
//
// A budget of zero means unbudgeted: nothing is ever dropped. The
// config template ships a real number, but a configuration that omits
// the field entirely must not silently resolve to "drop everything but
// must."
func selectForStage(candidates []budgetCandidate, budgetTokens int) budgetSelection {
	ordered := slices.Clone(candidates)
	slices.SortStableFunc(ordered, budgetOrder)

	sel := budgetSelection{Kept: []string{}, Dropped: []string{}}

	if budgetTokens <= 0 {
		for _, c := range ordered {
			sel.Kept = append(sel.Kept, c.id)
		}
		return sel
	}

	spent := 0
	mustBytes := 0
	for _, c := range ordered {
		if c.severity != "must" {
			continue
		}
		sel.Kept = append(sel.Kept, c.id)
		spent += tokensFor(c.bytes)
		mustBytes += c.bytes
	}
	if spent > budgetTokens {
		// The must set alone overflowed. Every must is still kept — the
		// overflow is reported in bytes, which is what guidance.md
		// prints and what doctor picks up, and no should or may can
		// possibly fit below.
		sel.MustOverflowBytes = mustBytes - budgetTokens*4
		for _, c := range ordered {
			if c.severity != "must" {
				sel.Dropped = append(sel.Dropped, c.id)
			}
		}
		return sel
	}

	// `may` drops first, then `should`: drop from the *end* of the
	// order — the lowest-priority entry — until what remains fits.
	//
	// Not "keep every entry that happens to fit": that would let a small
	// `may` survive while a larger `should` earlier in the order was
	// dropped, which is precisely the outcome "may drops first, then
	// should" forbids. Dropping strictly from the tail makes the kept
	// set a prefix of the total order, so severity is never inverted.
	rest := make([]budgetCandidate, 0, len(ordered))
	for _, c := range ordered {
		if c.severity != "must" {
			rest = append(rest, c)
		}
	}
	for _, c := range rest {
		spent += tokensFor(c.bytes)
	}
	cut := len(rest)
	for cut > 0 && spent > budgetTokens {
		cut--
		spent -= tokensFor(rest[cut].bytes)
	}
	for i, c := range rest {
		if i < cut {
			sel.Kept = append(sel.Kept, c.id)
		} else {
			sel.Dropped = append(sel.Dropped, c.id)
		}
	}
	return sel
}
