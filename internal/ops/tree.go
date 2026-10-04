package ops

import "io/fs"

// IsTreeInput reports whether a directory entry of type t is content of
// a source tree a rendering reads: regular files, and symlinks (which
// every addon's copy follows). FIFOs, sockets and devices are not — none
// is render input, and reading a FIFO blocks forever. TreeHash and each
// addon's copy use this one predicate, so what is hashed is exactly what
// is copied.
func IsTreeInput(t fs.FileMode) bool {
	return t.IsRegular() || t&fs.ModeSymlink != 0
}
