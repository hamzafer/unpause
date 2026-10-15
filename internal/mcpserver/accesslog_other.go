//go:build !(darwin || linux || freebsd || openbsd || netbsd || dragonfly)

package mcpserver

import (
	"fmt"
	"os"
)

// lockFile is a no-op where flock isn't available; the in-process mutex still holds.
func lockFile(string) (unlock func(), err error) { return func() {}, nil }

// openNoFollow opens path after checking it isn't a symlink. Without O_NOFOLLOW the
// check and the open are two steps.
func openNoFollow(path string, flag int, perm os.FileMode) (*os.File, error) {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symlink, refusing to write", path)
	}
	return os.OpenFile(path, flag, perm)
}
