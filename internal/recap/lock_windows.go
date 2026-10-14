package recap

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on f, waiting for it.
func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, new(windows.Overlapped))
}
