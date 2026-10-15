//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package mcpserver

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on path, creating it, until unlock is called.
func lockFile(path string) (unlock func(), err error) {
	f, err := openNoFollow(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// openNoFollow opens path, failing when it is a symlink.
func openNoFollow(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag|syscall.O_NOFOLLOW, perm)
}
