//go:build !windows

package exec

import (
	"os"
	"syscall"
)

// terminate asks p to exit: SIGTERM, so a dev server can shut down
// cleanly before Stop's grace runs out and it is killed.
func terminate(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
