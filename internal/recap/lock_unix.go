//go:build !windows

package recap

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive lock on f, waiting for it.
func lockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }
