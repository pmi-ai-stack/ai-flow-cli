//go:build unix

package render_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
)

// TestTreeHashSkipsNamedPipes proves a FIFO in a source tree is
// skipped rather than read — reading one blocks forever.
func TestTreeHashSkipsNamedPipes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := render.TreeHash(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600); err != nil {
		t.Skipf("mkfifo unsupported here: %v", err)
	}
	after, err := render.TreeHash(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("hash moved after adding a FIFO: %q → %q", before, after)
	}
}
