// Package standards is the pack format's loader, structural validator
// and sync mechanism — the content phase 12 makes fetchable, validatable
// and forkable. It knows nothing about layering, precedence, waivers,
// the token budget or guidance delivery: that is phase 13's resolver,
// built on top of this package's Manifest and Load, never inside it.
package standards
