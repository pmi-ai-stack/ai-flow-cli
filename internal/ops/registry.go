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

// DefaultRegistry is empty, always — not just "through phase 20." A
// real addon (terraform, phase 21 on) must import this package (for
// Addon, Target, Capabilities, ...), so this package can never import
// a real addon's own package back without a direct two-package import
// cycle. The registry this binary actually ships is built one layer
// up, in internal/cli (which already imports both internal/ops and
// each addon package): see internal/cli/ops.go's buildOpsRegistry.
// DefaultRegistry stays here only as the always-empty base every test
// that wants a deliberately addon-free registry can still name
// directly.
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
