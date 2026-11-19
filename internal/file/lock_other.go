//go:build !unix && !windows

package file

import (
	"errors"
	"os"
)

var errWouldBlock = errors.New("lock held elsewhere")

// Platforms without flock or LockFileEx (js/wasm, plan9) have no shared database files to
// protect, so the lock is advisory only.
func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) error { return nil }
