package ops

import "errors"

// ErrLiveCredentialsMissing is the error Addon.Drift and Addon.DryRun
// return when a target's environment requires live credentials the
// caller's environment does not have. It is a distinct sentinel,
// checked with errors.Is, rather than an *envelope.Error carrying
// "tool_failed" — the two conditions need different CLI reactions
// (exit 4 live_credentials_missing, asking the developer to provide
// credentials or go offline, versus exit 5 tool_failed, which halts).
// An addon must say so explicitly by returning this error; a caller
// never infers "missing credentials" from a generic failure.
//
// This is a bare sentinel rather than a fixed *envelope.Error (the
// shape internal/ops/envs's own env_ambiguous condition returns
// directly, since it has exactly one CLI reaction) because the two
// callers here need different envelope errors for the identical
// condition: internal/cli/ops_drift.go degrades it to drift: "offline"
// with no envelope error at all, while internal/cli/ops_dryrun.go
// raises live_credentials_missing only when --offline was not already
// passed. A single fixed envelope.Error could not serve both reactions
// from one Addon call.
var ErrLiveCredentialsMissing = errors.New("ops: live credentials missing")
