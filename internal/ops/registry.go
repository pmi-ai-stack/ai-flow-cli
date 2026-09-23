package ops

import "slices"

// Registry is the enumeration of every addon this binary knows about —
// `ops addons` prints exactly Registry.Addons(), in registration
// order. It is a plain value, not a package-level singleton with
// hidden mutation: production code uses DefaultRegistry(), tests build
// their own with NewRegistry(fixtureaddon.New()) so a test's addon
// list can never leak into another test or into production.
type Registry struct {
	addons []Addon
}

// NewRegistry builds a Registry from addons, in the given order.
func NewRegistry(addons ...Addon) *Registry {
	return &Registry{addons: slices.Clone(addons)}
}

// DefaultRegistry is the registry this binary actually ships. It is
// empty through phase 20 — no addon implementation exists before
// phase 21 (terraform) — so `ops addons`/`ops detect` correctly report
// zero addons today, not a stub answer. Phases 21, 22, 26 and 27 each
// append their own addon here in the same PR that builds it.
func DefaultRegistry() *Registry {
	return NewRegistry()
}

// Addons returns every registered addon, in registration order. The
// caller's to keep — always a fresh copy of the slice.
func (r *Registry) Addons() []Addon {
	return slices.Clone(r.addons)
}

// Lookup returns the addon named name, and whether one was found.
func (r *Registry) Lookup(name string) (Addon, bool) {
	for _, a := range r.addons {
		if a.Name() == name {
			return a, true
		}
	}
	return nil, false
}
