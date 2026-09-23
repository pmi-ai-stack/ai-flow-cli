package standards

import "slices"

// Applies reports whether an applies_to narrowing matches a
// resolution's languages, addons and one stage.
//
// Every dimension is independent and every non-empty one must
// intersect: an entry narrowed to `{languages: ["go"], stages:
// ["implement"]}` applies only when Go is detected *and* the stage is
// implement. An empty dimension matches everything, so the zero
// AppliesTo applies everywhere — the reading the pack format's own
// "applies_to does the matching" line implies, since a pack author who
// writes no narrowing at all means "always."
func Applies(a AppliesTo, languages, addons []string, stage string) bool {
	return intersects(a.Languages, languages) &&
		intersects(a.Addons, addons) &&
		(len(a.Stages) == 0 || slices.Contains(a.Stages, stage))
}

// AppliesToAnyStage is Applies with the stage dimension ignored —
// "does this entry apply to this repository at all, in any stage." It
// is what the check set is filtered by: a check's stage narrowing
// selects when it runs, not whether it is part of the resolution.
func AppliesToAnyStage(a AppliesTo, languages, addons []string) bool {
	return intersects(a.Languages, languages) && intersects(a.Addons, addons)
}

// intersects reports whether narrowing is empty (matches everything)
// or shares at least one value with have.
func intersects(narrowing, have []string) bool {
	if len(narrowing) == 0 {
		return true
	}
	for _, n := range narrowing {
		if slices.Contains(have, n) {
			return true
		}
	}
	return false
}
