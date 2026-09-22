// Package packs holds the ai-flow-cli embedded reference standards
// pack — the org layer used whenever config.standards.sources is
// empty. This package embeds the raw pack content only;
// internal/standards.Export builds the loader/copy logic on top of FS,
// the same split schema/embed.go and internal/schema already use for
// the embedded JSON Schemas.
package packs

import "embed"

// FS embeds every file under reference/, dotfiles included (all:) —
// several per-language checks/ directories carry a .editorconfig that
// a plain //go:embed pattern would otherwise silently drop.
//
//go:embed all:reference
var FS embed.FS
