//go:build windows

package exec

import "os"

// terminate asks p to exit. Windows has no SIGTERM: os.Process.Signal
// supports only Kill there, and any other signal is an error, which
// used to make every Stop fail and leave the process running. So the
// request to exit is the kill itself, and Stop's grace is unused.
func terminate(p *os.Process) error {
	return p.Kill()
}
