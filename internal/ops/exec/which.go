package exec

import osexec "os/exec"

// Which reports whether name is found on PATH, and where. It is
// doctor's (phase 8) only subprocess-adjacent need — checking a tool's
// presence, not running it — and os/exec.LookPath itself starts no
// subprocess, so calling it from here stays true to the spirit of
// TestNoOsExecOutsideExecPackage (every os/exec symbol confined to this
// one package) without adding an allowlist row: there is nothing here
// for Allowlist.Match to guard.
func Which(name string) (path string, found bool) {
	p, err := osexec.LookPath(name)
	if err != nil {
		return "", false
	}
	return p, true
}
