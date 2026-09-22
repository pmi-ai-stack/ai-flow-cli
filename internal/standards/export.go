package standards

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/packs"
)

// Export writes the embedded reference pack out to dir, byte for byte
// — the fork starting point standards.json's own org layer uses when
// config.standards.sources is empty. It refuses a dir that already has
// entries in it — the same "refuse, do not overwrite" shape
// internal/config.Init uses for an existing config.json — rather than
// silently interleaving with (or clobbering) a developer's
// fork-in-progress.
func Export(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case err == nil && len(entries) > 0:
		return &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("%s is not empty", dir),
			Details: []string{"export into a new or empty directory"},
		}
	case err == nil, os.IsNotExist(err):
		// expected — dir is empty or doesn't exist yet, either is the
		// write path (os.CopyFS below creates it if absent)
	default:
		return fmt.Errorf("standards: read %s: %w", dir, err)
	}

	sub, err := fs.Sub(packs.FS, "reference")
	if err != nil {
		// packs.FS always embeds reference/ — a failure here is a
		// broken embed, a programmer error, not a possible runtime
		// state (04-errors.md rule 9).
		panic(fmt.Sprintf("standards: embedded packs.FS has no reference/: %v", err))
	}
	if err := os.CopyFS(dir, sub); err != nil {
		return fmt.Errorf("standards: export to %s: %w", dir, err)
	}
	return nil
}
