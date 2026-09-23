package ops

// Env is a named live target from config.ops.environments with a tier.
// The zero value (Name == "") is the unbound environment: no addon
// binding exists at all, so its actions still score (at tier
// "nonprod") and config.ops.target_map can still bind it explicitly.
type Env struct {
	Name string
	Tier string
}
