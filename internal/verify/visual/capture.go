// Package visual starts a repository's dev server, captures a
// screenshot per screen, and stops the server again — `visual run`'s
// mechanism. `visual judge` (internal/cli) reads the screenshots this
// package writes and records a verdict; nothing here judges anything.
package visual

import "context"

// CaptureRequest is one screenshot to take.
type CaptureRequest struct {
	URL        string
	Viewport   string
	OutputPath string
}

// CaptureResult is what a capture produced.
type CaptureResult struct {
	Path string
}

// Capturer takes one screenshot per call. ScriptCapturer is the one
// production implementation (the plan's design decision 6) — a second
// implementation (an embedded headless browser, the spec's own open
// question) is additive: a new type satisfying this same two-method
// interface, not a rewrite of anything that depends on it.
type Capturer interface {
	Capture(ctx context.Context, req CaptureRequest) (CaptureResult, error)
}
